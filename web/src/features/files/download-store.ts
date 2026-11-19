// Only this private OPFS directory is managed; user destinations are never deleted.
export const stagingRoot = "okserver-downloads-v1";
export interface DownloadRecord {
  id: string;
  path: string;
  name: string;
  size: number;
  etag: string;
  hash: string;
  hashes: string[];
}
export async function downloadRoot() {
  return (await navigator.storage.getDirectory()).getDirectoryHandle(stagingRoot, { create: true });
}
export async function stagingID(path: string) {
  const bytes = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(path));
  return Array.from(new Uint8Array(bytes), (b) => b.toString(16).padStart(2, "0")).join("");
}
export async function readRecord(directory: FileSystemDirectoryHandle): Promise<DownloadRecord> {
  const record = JSON.parse(
    await (await (await directory.getFileHandle("manifest.json")).getFile()).text(),
  ) as DownloadRecord;
  if (
    !record ||
    typeof record.path !== "string" ||
    typeof record.name !== "string" ||
    typeof record.etag !== "string" ||
    !Number.isSafeInteger(record.size) ||
    record.size < 0 ||
    !/^[a-f0-9]{64}$/.test(record.hash) ||
    !Array.isArray(record.hashes) ||
    record.hashes.length > 1_000_000 ||
    record.hashes.some(
      (hash) => typeof hash !== "string" || (hash !== "" && !/^[a-f0-9]{64}$/.test(hash)),
    )
  )
    throw new SyntaxError("Invalid staging manifest");
  return record;
}
export async function saveRecord(directory: FileSystemDirectoryHandle, record: DownloadRecord) {
  const writer = await (
    await directory.getFileHandle("manifest.json", { create: true })
  ).createWritable();
  try {
    await writer.write(JSON.stringify(record));
    await writer.close();
  } catch (error) {
    await writer.abort().catch(() => {});
    throw error;
  }
}
export async function stagedDownloads() {
  const root = await downloadRoot();
  const records: Array<DownloadRecord & { bytes: number }> = [];
  for await (const [id, entry] of root.entries()) {
    if (entry.kind !== "directory") continue;
    try {
      const directory = await root.getDirectoryHandle(id);
      const record = await readRecord(directory);
      let bytes = 0;
      for await (const [name, item] of directory.entries())
        if (/^\d+\.part$/.test(name) && item.kind === "file")
          bytes += (await (await directory.getFileHandle(name)).getFile()).size;
      records.push({ ...record, id, bytes });
    } catch {
      records.push({ id, path: id, name: id, size: 0, etag: "", hash: "", hashes: [], bytes: 0 });
    }
  }
  return records;
}
export function withDownloadLock<T>(
  id: string,
  action: () => Promise<T>,
  signal?: AbortSignal,
): Promise<T> {
  return navigator.locks.request(`okserver-download:${id}`, { signal }, action);
}
export async function removeStaging(id: string) {
  if (!/^[a-f0-9]{64}$/.test(id)) throw new Error("Invalid staging ID");
  await (await downloadRoot()).removeEntry(id, { recursive: true }).catch((error: unknown) => {
    if (!(error instanceof DOMException && error.name === "NotFoundError")) throw error;
  });
}
