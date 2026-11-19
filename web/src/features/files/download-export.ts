export interface DiskWriter {
  write(data: { type: "write"; position: number; data: ArrayBuffer }): Promise<void>;
  truncate(size: number): Promise<void>;
  close(): Promise<void>;
  abort(): Promise<void>;
}
export interface DiskHandle {
  createWritable(options: { keepExistingData: boolean }): Promise<DiskWriter>;
}

// Caller must verify the immutable staged Blob first. The target's transactional
// writer is opened only here and aborted on every unsuccessful export.
export async function exportVerified(handle: DiskHandle, staged: Blob, signal: AbortSignal) {
  signal.throwIfAborted();
  const writer = await handle.createWritable({ keepExistingData: false });
  try {
    for (let position = 0; position < staged.size; position += 8 << 20) {
      signal.throwIfAborted();
      await writer.write({
        type: "write",
        position,
        data: await staged.slice(position, position + (8 << 20)).arrayBuffer(),
      });
    }
    signal.throwIfAborted();
    await writer.truncate(staged.size);
    await writer.close();
  } catch (error) {
    await writer.abort().catch(() => {});
    throw error;
  }
}
