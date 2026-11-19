import { createSHA256, type IHasher } from "hash-wasm";

type HashRequest =
  | { id: number; type: "reset"; file: Blob; offset: number }
  | { id: number; type: "process"; start: number; end: number }
  | { id: number; type: "digest" };

let file: Blob | undefined;
let whole: IHasher | undefined;
const blockSize = 4 << 20;

self.onmessage = async (event: MessageEvent<HashRequest>) => {
  const request = event.data;
  try {
    if (request.type === "reset") {
      if (request.offset < 0 || request.offset > request.file.size)
        throw new Error("Invalid resume offset");
      file = request.file;
      whole = await createSHA256();
      for (let start = 0; start < request.offset; start += blockSize) {
        const end = Math.min(start + blockSize, request.offset);
        whole.update(new Uint8Array(await file.slice(start, end).arrayBuffer()));
        self.postMessage({ id: request.id, type: "progress", offset: end });
      }
      self.postMessage({ id: request.id, type: "ready" });
      return;
    }

    if (!whole || !file) throw new Error("Hash worker is not initialized");
    if (request.type === "process") {
      const { start, end } = request;
      if (start < 0 || end <= start || end > file.size || end - start > 8 << 20)
        throw new Error("Invalid chunk range");
      const buffer = await file.slice(start, end).arrayBuffer();
      const bytes = new Uint8Array(buffer);
      whole.update(bytes);
      const chunk = await createSHA256();
      chunk.update(bytes);
      self.postMessage(
        { id: request.id, type: "chunk", buffer, checksum: chunk.digest() },
        { transfer: [buffer] },
      );
      return;
    }

    const snapshot = whole.save();
    const digest = whole.digest();
    whole.load(snapshot);
    self.postMessage({ id: request.id, type: "digest", digest });
  } catch (error) {
    self.postMessage({
      id: request.id,
      type: "error",
      message: error instanceof Error ? error.message : String(error),
    });
  }
};
