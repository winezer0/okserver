import { retryFetch } from "../../lib/api/retry-fetch";
import { delay } from "../uploads/transfer";

class IncompleteRange extends Error {}
async function readRange(response: Response, length: number) {
  const reader = response.body?.getReader();
  if (!reader) throw new IncompleteRange("Missing download body");
  const bytes = new Uint8Array(length);
  let offset = 0;
  try {
    for (;;) {
      const next = await reader.read();
      if (next.done) break;
      if (next.value.length > length - offset) throw new Error("Oversized download range");
      bytes.set(next.value, offset);
      offset += next.value.length;
    }
    if (offset !== length) throw new IncompleteRange("Incomplete download range");
    return bytes.buffer;
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

export async function downloadRange(
  url: string,
  start: number,
  end: number,
  size: number,
  etag: string,
  hash: string,
  signal: AbortSignal,
): Promise<ArrayBuffer> {
  for (let attempt = 0; ; attempt++) {
    const response = await retryFetch(url, {
      signal,
      headers: { Range: `bytes=${start}-${end}`, "If-Range": etag },
    });
    if (
      response.status !== 206 ||
      response.headers.get("Content-Range") !== `bytes ${start}-${end}/${size}` ||
      response.headers.get("ETag") !== etag ||
      response.headers.get("X-File-SHA256") !== hash
    ) {
      await response.body?.cancel();
      throw new Error(`Download version or range changed (HTTP ${response.status})`);
    }
    try {
      return await readRange(response, end - start + 1);
    } catch (error) {
      if (
        attempt === 5 ||
        signal.aborted ||
        !(error instanceof TypeError || error instanceof IncompleteRange)
      )
        throw error;
      await delay(Math.min(30000, 500 * 2 ** attempt) * (0.5 + Math.random()), signal);
    }
  }
}
