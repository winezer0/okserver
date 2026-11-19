import { describe, expect, it, vi } from "vitest";
import { sha256 } from "hash-wasm";
import { hashFile } from "./transfer";

const blob = (bytes: Uint8Array) => new globalThis.Blob([bytes as BlobPart]);
describe("streaming integrity and resumable transfer", () => {
  it("hashes bounded blocks including empty and partial blocks", async () => {
    // jsdom's Blob lacks arrayBuffer; use a native Blob as supplied by real browsers.
    const { Blob } = await import("node:buffer");
    for (const size of [0, 3, 4 * 1024 * 1024, 4 * 1024 * 1024 + 71]) {
      const bytes = new Uint8Array(size).fill(17);
      expect(await hashFile(new Blob([bytes]) as unknown as globalThis.Blob)).toBe(
        await sha256(bytes),
      );
    }
  });
  it("cancels checksum work before reading", async () => {
    const signal = AbortSignal.abort();
    await expect(hashFile(blob(new Uint8Array(3)), signal)).rejects.toMatchObject({
      name: "AbortError",
    });
  });
  it("hashes in a module worker, reports progress and terminates it", async () => {
    const terminate = vi.fn();
    class FakeWorker {
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: ErrorEvent) => void) | null = null;
      postMessage(message: { id: number; type: string }) {
        queueMicrotask(async () => {
          if (message.type === "reset") {
            this.onmessage?.({
              data: { id: message.id, type: "progress", offset: 7 },
            } as MessageEvent);
            this.onmessage?.({ data: { id: message.id, type: "ready" } } as MessageEvent);
          } else if (message.type === "digest") {
            this.onmessage?.({
              data: { id: message.id, type: "digest", digest: await sha256("worker") },
            } as MessageEvent);
          }
        });
      }
      terminate = terminate;
    }
    vi.stubGlobal("Worker", FakeWorker);
    const progress = vi.fn();
    await expect(hashFile(new Blob(["worker"]), undefined, progress)).resolves.toBe(
      await sha256("worker"),
    );
    expect(progress).toHaveBeenCalledWith(7);
    expect(terminate).toHaveBeenCalledOnce();
    vi.unstubAllGlobals();
  });
});
