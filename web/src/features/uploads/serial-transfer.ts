import { APIError, apiErrorFromResponse, apiURL, cancelUpload, isRetryable } from "../../lib/api";
import { retryFetch } from "../../lib/api/retry-fetch";
import { UploadHasher, delay, type ProgressListener } from "./transfer";

export async function serialUpload(
  file: File,
  directory: string,
  report: ProgressListener,
  signal: AbortSignal,
  confirmOverwrite: (path: string) => boolean | Promise<boolean> = () => false,
) {
  const path = directory ? `${directory}/${file.name}` : file.name;
  const url = apiURL(`/upload/${path.split("/").map(encodeURIComponent).join("/")}`);
  const downloadURL = apiURL(`/download/${path.split("/").map(encodeURIComponent).join("/")}`);
  const currentETag = async () => {
    const response = await retryFetch(downloadURL, { method: "HEAD", signal });
    if (response.status === 404) return undefined;
    if (!response.ok) throw new APIError(response.status, `HTTP ${response.status}`);
    const etag = response.headers.get("ETag");
    if (!etag) throw new APIError(500, "The destination did not provide an ETag");
    return etag;
  };
  let overwriteETag = await currentETag();
  if (overwriteETag && !(await confirmOverwrite(path)))
    throw new DOMException("Overwrite canceled", "AbortError");
  const tokenKey = `okserver:upload-token:${location.origin}:${path}`;
  let token = Array.from(crypto.getRandomValues(new Uint8Array(16)), (value) =>
    value.toString(16).padStart(2, "0"),
  ).join("");
  try {
    token = localStorage.getItem(tokenKey) || token;
    localStorage.setItem(tokenKey, token);
  } catch {
    /* The token still isolates this transfer. */
  }
  let chunkSize = 8 << 20;
  let sessionId: string | undefined;
  const query = async (prefix = false) => {
    const target = new URL(url, location.origin);
    if (prefix) target.searchParams.set("prefix", "sha256");
    const response = await retryFetch(target.toString(), {
      method: "HEAD",
      headers: { "Upload-Token": token },
      signal,
    });
    const limit = Number(response.headers.get("Upload-Chunk-Size"));
    if (Number.isSafeInteger(limit) && limit > 0) chunkSize = Math.min(chunkSize, limit);
    if (response.status === 404) return undefined;
    if (!response.ok) throw new APIError(response.status, `HTTP ${response.status}`);
    const phase = response.headers.get("Upload-State");
    if (phase === "canceled") return undefined;
    const offset = Number(response.headers.get("Upload-Offset"));
    const size = Number(response.headers.get("Upload-Length"));
    if (phase === "failed") {
      sessionId = response.headers.get("Upload-ID") || undefined;
      return {
        offset: Number.isSafeInteger(offset) && offset >= 0 ? offset : 0,
        prefix: response.headers.get("Upload-Prefix-SHA256"),
        phase,
        checksum: response.headers.get("X-File-SHA256"),
        errorCode: response.headers.get("Upload-Error-Code") || undefined,
      };
    }
    if (!Number.isSafeInteger(offset) || offset < 0 || offset > file.size || size !== file.size)
      throw new Error("SESSION_MISMATCH");
    sessionId = response.headers.get("Upload-ID") || undefined;
    return {
      offset,
      prefix: response.headers.get("Upload-Prefix-SHA256"),
      phase,
      checksum: response.headers.get("X-File-SHA256"),
      errorCode: response.headers.get("Upload-Error-Code") || undefined,
    };
  };
  const completed = (checksum: string, actual: string | null) => {
    if (checksum !== actual) throw new Error("INTEGRITY_CHECK_FAILED");
    report({ offset: file.size, phase: "completed", sessionId });
  };
  const awaitCompletion = async (checksum: string) => {
    report({ offset: file.size, phase: "verifying", sessionId });
    for (let poll = 0; poll < 86400; poll++) {
      const state = await query();
      if (!state) throw new Error("INTEGRITY_CHECK_FAILED");
      if (state.phase === "completed") {
        completed(checksum, state.checksum);
        return;
      }
      if (state.phase === "failed") {
        const code = state.errorCode || "upload_failed";
        throw new APIError(code === "destination_changed" ? 412 : 500, code, 0, code);
      }
      await delay(1000, signal);
    }
    throw new Error("Upload verification timed out");
  };
  let state = await query(true);
  if (state?.phase === "failed" || (overwriteETag && state?.phase === "completed")) {
    state = undefined;
    sessionId = undefined;
  }
  let offset = state?.offset || 0;
  const hasher = await UploadHasher.create(file, offset, signal, (position) =>
    report({ offset: position, phase: "hashing", sessionId }),
  );
  try {
    if (state && (await hasher.digest()) !== state.prefix) throw new Error("SESSION_MISMATCH");
    if (state && offset === file.size) {
      await awaitCompletion(await hasher.digest());
      return;
    }
    do {
      const end = Math.min(offset + chunkSize, file.size);
      const chunk =
        end > offset
          ? await hasher.process(offset, end)
          : { body: new ArrayBuffer(0), checksum: "" };
      const final = end === file.size;
      const checksum = final ? await hasher.digest() : undefined;
      let accepted = false;
      let conflictConfirmations = 0;
      for (let attempt = 0; attempt < 6; attempt++) {
        if (signal.aborted) throw new DOMException("Paused", "AbortError");
        try {
          const headers: Record<string, string> = { "Upload-Token": token };
          if (file.size) {
            headers["Content-Range"] = `bytes ${offset}-${end - 1}/${file.size}`;
            headers["X-Chunk-SHA256"] = chunk.checksum;
          }
          if (checksum) headers["X-File-SHA256"] = checksum;
          if (overwriteETag) headers["If-Match"] = overwriteETag;
          const requestURL = new URL(url, location.origin);
          if (overwriteETag) requestURL.searchParams.set("overwrite", "true");
          const response = await fetch(requestURL.toString(), {
            method: file.size ? "PUT" : "POST",
            headers,
            body: chunk.body,
            signal,
          });
          if (!response.ok) throw await apiErrorFromResponse(response);
          sessionId = response.headers.get("Upload-ID") || sessionId;
          if (response.status === 202 && checksum) {
            await awaitCompletion(checksum);
            return;
          }
          if (Number(response.headers.get("Upload-Offset")) !== end)
            throw new Error("INVALID_UPLOAD_OFFSET");
          if (checksum) completed(checksum, response.headers.get("X-File-SHA256"));
          accepted = true;
          break;
        } catch (error) {
          if (error instanceof APIError && error.code === "destination_exists") {
            if (++conflictConfirmations > 3) throw error;
            overwriteETag = await currentETag();
            if (!overwriteETag || !(await confirmOverwrite(path)))
              throw new DOMException("Overwrite canceled", "AbortError");
            attempt--;
            continue;
          }
          if (error instanceof APIError && error.code === "destination_changed") {
            if (sessionId) await cancelUpload(sessionId).catch(() => undefined);
            throw error;
          }
          if (
            error instanceof Error &&
            ["INTEGRITY_CHECK_FAILED", "INVALID_UPLOAD_OFFSET", "SESSION_MISMATCH"].includes(
              error.message,
            )
          )
            throw error;
          if (
            (!isRetryable(error, true) && !(error instanceof APIError && error.status === 422)) ||
            attempt === 5
          )
            throw error;
          state = await query();
          if (state?.phase === "failed") {
            const code = state.errorCode || "upload_failed";
            throw new APIError(code === "destination_changed" ? 412 : 500, code, 0, code);
          }
          if (state?.offset === end) {
            if (checksum) {
              await awaitCompletion(checksum);
              return;
            }
            accepted = true;
            break;
          }
          if ((state?.offset ?? 0) !== offset) throw new Error("SESSION_MISMATCH");
          report({ offset, phase: "retrying", retry: attempt + 1, sessionId });
          await delay(
            Math.max(
              Math.min(30000, 500 * 2 ** attempt) * (0.5 + Math.random()),
              error instanceof APIError ? error.retryAfterMS : 0,
            ),
            signal,
          );
        }
      }
      if (!accepted) throw new Error("Upload retry limit exceeded");
      offset = end;
      report({ offset, phase: final ? "completed" : "uploading", sessionId });
    } while (offset < file.size);
  } finally {
    hasher.dispose();
  }
}
