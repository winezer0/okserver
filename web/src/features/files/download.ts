import { apiURL } from "../../lib/api";
import { rememberDownload } from "./transfer-records";
import {
  downloadRoot,
  stagingID,
  readRecord,
  saveRecord,
  removeStaging,
  withDownloadLock,
} from "./download-store";
import { retryFetch } from "../../lib/api/retry-fetch";
import { hashFile } from "../uploads/transfer";
import { downloadRange } from "./download-protocol";
import { exportVerified, type DiskHandle } from "./download-export";
type Picker = (options: { suggestedName: string }) => Promise<DiskHandle>;
const jobs = new Map<string, { controller: AbortController; done: Promise<boolean> }>();
const channel =
  typeof window.BroadcastChannel === "function"
    ? new BroadcastChannel("okserver-download-cleanup")
    : undefined;
channel?.addEventListener("message", (event: MessageEvent<unknown>) => {
  if (typeof event.data === "string") jobs.get(event.data)?.controller.abort();
});
export const isDownloadActive = (id: string) => jobs.has(id);
export async function pauseDownload(id: string) {
  const job = jobs.get(id);
  job?.controller.abort();
  await job?.done.catch(() => {});
}
export async function clearDownload(id: string) {
  channel?.postMessage(id);
  const job = jobs.get(id);
  job?.controller.abort();
  await job?.done.catch(() => {});
  await withDownloadLock(id, () => removeStaging(id));
}

// Returns false when native downloads are the only supported disk-safe option.
export async function downloadVerified(
  path: string,
  name: string,
  progress?: (text: string) => void,
  signal?: AbortSignal,
): Promise<boolean> {
  const picker = (window as unknown as { showSaveFilePicker?: Picker }).showSaveFilePicker;
  if (!picker || !navigator.storage?.getDirectory || !navigator.locks) return false;
  const handle = await picker({ suggestedName: name });
  const id = await stagingID(path);
  const controller = new AbortController();
  if (jobs.has(id)) throw new Error("Download already active");
  const cancel = () => controller.abort();
  signal?.addEventListener("abort", cancel, { once: true });
  if (signal?.aborted) controller.abort();
  const work = withDownloadLock(
    id,
    async () => {
      const url = apiURL(`/download/${path.split("/").map(encodeURIComponent).join("/")}`);
      const head = await retryFetch(url, { method: "HEAD", signal: controller.signal });
      if (!head.ok) throw new Error(`HTTP ${head.status}`);
      const size = Number(head.headers.get("X-File-Size"));
      const hash = head.headers.get("X-File-SHA256");
      const etag = head.headers.get("ETag");
      if (
        !Number.isSafeInteger(size) ||
        size < 0 ||
        !hash ||
        !/^[a-f0-9]{64}$/.test(hash) ||
        !etag ||
        etag.startsWith("W/")
      )
        throw new Error("Invalid download metadata");
      const chunk = 8 << 20;
      const count = Math.ceil(size / chunk);
      if (count > 1_000_000) throw new Error("Download contains too many ranges");
      let done: boolean[] = Array(count).fill(false);
      let hashes: string[] = Array(count).fill("");
      const root = await downloadRoot();
      let directory = await root.getDirectoryHandle(id, { create: true });
      const parts: Blob[] = Array(count);
      try {
        const saved = (await readRecord(directory)) as {
          etag?: string;
          hash?: string;
          hashes?: string[];
        } | null;
        if (saved?.etag === etag && saved.hash === hash && saved.hashes?.length === count) {
          hashes = saved.hashes;
          // Browser writable streams commit on close. Verify durable bytes instead of
          // trusting a bitmap saved before a tab crash or stream abort.
          for (let n = 0; n < count; n++)
            if (hashes[n]) {
              try {
                const local = await (await directory.getFileHandle(`${n}.part`)).getFile();
                done[n] =
                  local.size === Math.min(chunk, size - n * chunk) &&
                  (await hashFile(local, controller.signal)) === hashes[n];
                if (done[n]) parts[n] = local;
              } catch (error) {
                if (!(error instanceof DOMException && error.name === "NotFoundError")) throw error;
              }
            }
        } else {
          await removeStaging(id);
          directory = await root.getDirectoryHandle(id, { create: true });
        }
      } catch (error) {
        if (
          !(error instanceof SyntaxError) &&
          !(error instanceof DOMException && error.name === "NotFoundError")
        )
          throw error;
      }
      let index = 0;
      let writes = Promise.resolve();
      const save = () => saveRecord(directory, { id, path, name, size, etag, hash, hashes });
      rememberDownload({ id, path, name, size, completed: false });
      await save();
      try {
        const results = await Promise.allSettled(
          Array.from({ length: 4 }, async () => {
            try {
              while (index < count) {
                const current = index++;
                if (done[current]) continue;
                const start = current * chunk;
                const end = Math.min(start + chunk, size) - 1;
                const buffer = await downloadRange(
                  url,
                  start,
                  end,
                  size,
                  etag,
                  hash,
                  controller.signal,
                );
                const checksum = await hashFile(new Blob([buffer]), controller.signal);
                writes = writes.then(async () => {
                  controller.signal.throwIfAborted();
                  const part = await directory.getFileHandle(`${current}.part`, { create: true });
                  const output = await part.createWritable();
                  try {
                    await output.write(buffer);
                    controller.signal.throwIfAborted();
                    await output.close();
                  } catch (error) {
                    await output.abort().catch(() => {});
                    throw error;
                  }
                  parts[current] = await part.getFile();
                  done[current] = true;
                  hashes[current] = checksum;
                  await save();
                  progress?.(`${done.filter(Boolean).length}/${count}`);
                });
                await writes;
              }
            } catch (error) {
              controller.abort();
              throw error;
            }
          }),
        );
        const failed = results.find((result) => result.status === "rejected");
        if (failed?.status === "rejected") throw failed.reason;
        await writes;
        progress?.("SHA-256…");
        const downloaded = new Blob(parts);
        if (downloaded.size !== size || (await hashFile(downloaded, controller.signal)) !== hash) {
          throw new Error("SHA-256 verification failed");
        }
        await exportVerified(handle, downloaded, controller.signal);
        await removeStaging(id);
        rememberDownload({ id, path, name, size, completed: true });
        progress?.("SHA-256 ✓");
        return true;
      } catch (error) {
        rememberDownload({
          id,
          path,
          name,
          size,
          completed: false,
          error:
            error instanceof DOMException && error.name === "AbortError"
              ? undefined
              : String(error),
        });
        controller.abort();
        await writes.catch(() => {});
        throw error;
      }
    },
    controller.signal,
  );
  jobs.set(id, { controller, done: work });
  try {
    return await work;
  } finally {
    signal?.removeEventListener("abort", cancel);
    jobs.delete(id);
  }
}
