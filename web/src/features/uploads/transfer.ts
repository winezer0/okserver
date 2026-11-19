import type { IHasher } from "hash-wasm";

export type Phase =
  | "waiting"
  | "hashing"
  | "uploading"
  | "retrying"
  | "verifying"
  | "completed"
  | "paused"
  | "failed";
export interface Progress {
  offset: number;
  phase: Phase;
  sessionId?: string;
  retry?: number;
}
export type ProgressListener = (progress: Progress) => void;
const maxChunkSize = 8 << 20;
const hashBlockSize = 4 << 20;
const createSHA256Hasher = async () => (await import("hash-wasm")).createSHA256();

interface WorkerResponse {
  id: number;
  type: "progress" | "ready" | "chunk" | "digest" | "error";
  offset?: number;
  buffer?: ArrayBuffer;
  checksum?: string;
  digest?: string;
  message?: string;
}

function abort(signal?: AbortSignal) {
  if (signal?.aborted) throw new DOMException("Paused", "AbortError");
}

export function delay(ms: number, signal?: AbortSignal): Promise<void> {
  abort(signal);
  return new Promise((resolve, reject) => {
    const onAbort = () => {
      clearTimeout(timer);
      signal?.removeEventListener("abort", onAbort);
      reject(new DOMException("Paused", "AbortError"));
    };
    const timer = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

export class UploadHasher {
  private worker?: Worker;
  private hasher?: IHasher;
  private sequence = 0;
  private pending = new Map<
    number,
    { resolve: (response: WorkerResponse) => void; reject: (error: Error) => void }
  >();

  private constructor(
    private readonly file: Blob,
    private readonly signal?: AbortSignal,
    private readonly onProgress: (offset: number) => void = () => {},
  ) {
    if (typeof Worker !== "undefined") {
      this.worker = new Worker(new URL("./hash.worker.ts", import.meta.url), { type: "module" });
      this.worker.onmessage = (event: MessageEvent<WorkerResponse>) => {
        const response = event.data;
        if (response.type === "progress") {
          this.onProgress(response.offset || 0);
          return;
        }
        const pending = this.pending.get(response.id);
        if (!pending) return;
        this.pending.delete(response.id);
        if (response.type === "error")
          pending.reject(new Error(response.message || "Checksum failed"));
        else pending.resolve(response);
      };
      this.worker.onerror = (event) =>
        this.failPending(new Error(event.message || "Checksum worker failed"));
      this.signal?.addEventListener("abort", this.onAbort, { once: true });
    }
  }

  private onAbort = () => this.failPending(new DOMException("Paused", "AbortError"));

  static async create(
    file: Blob,
    offset: number,
    signal?: AbortSignal,
    onProgress?: (offset: number) => void,
  ) {
    const hasher = new UploadHasher(file, signal, onProgress);
    try {
      abort(signal);
      if (hasher.worker) {
        await hasher.request({ type: "reset", file, offset });
      } else {
        hasher.hasher = await createSHA256Hasher();
        for (let start = 0; start < offset; start += hashBlockSize) {
          abort(signal);
          const end = Math.min(start + hashBlockSize, offset);
          hasher.hasher.update(new Uint8Array(await file.slice(start, end).arrayBuffer()));
          onProgress?.(end);
          await delay(0, signal);
        }
      }
      return hasher;
    } catch (error) {
      hasher.dispose();
      throw error;
    }
  }

  async process(start: number, end: number) {
    abort(this.signal);
    if (end <= start || end - start > maxChunkSize || end > this.file.size)
      throw new Error("Invalid chunk range");
    if (this.worker) {
      const response = await this.request({ type: "process", start, end });
      if (!response.buffer || !response.checksum)
        throw new Error("Checksum worker returned an invalid chunk");
      return { body: response.buffer, checksum: response.checksum };
    }
    const body = await this.file.slice(start, end).arrayBuffer();
    const bytes = new Uint8Array(body);
    this.hasher!.update(bytes);
    const chunk = await createSHA256Hasher();
    chunk.update(bytes);
    return { body, checksum: chunk.digest() };
  }

  async digest() {
    abort(this.signal);
    if (this.worker) {
      const response = await this.request({ type: "digest" });
      if (!response.digest) throw new Error("Checksum worker returned an invalid digest");
      return response.digest;
    }
    const snapshot = this.hasher!.save();
    const result = this.hasher!.digest();
    this.hasher!.load(snapshot);
    return result;
  }

  dispose() {
    this.signal?.removeEventListener("abort", this.onAbort);
    this.worker?.terminate();
    this.worker = undefined;
    this.hasher = undefined;
    this.pending.clear();
  }

  private request(message: Record<string, unknown>): Promise<WorkerResponse> {
    abort(this.signal);
    const id = ++this.sequence;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      try {
        this.worker!.postMessage({ ...message, id });
      } catch (error) {
        this.pending.delete(id);
        reject(error);
      }
    });
  }

  private failPending(error: Error) {
    this.worker?.terminate();
    this.worker = undefined;
    for (const pending of this.pending.values()) pending.reject(error);
    this.pending.clear();
  }
}

export async function hashFile(
  file: Blob,
  signal?: AbortSignal,
  onProgress: (offset: number) => void = () => {},
) {
  const hasher = await UploadHasher.create(file, file.size, signal, onProgress);
  try {
    return await hasher.digest();
  } finally {
    hasher.dispose();
  }
}
