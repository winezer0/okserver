import { beforeEach, expect, it, vi } from "vitest";
import { Blob, File } from "node:buffer";
import { sha256 } from "hash-wasm";
import { downloadVerified, clearDownload } from "./download";
import { stagedDownloads, downloadRoot } from "./download-store";

// Transactional disk double: bytes become visible only on close, as in OPFS.
class Directory {
  kind = "directory";
  children = new Map<string, Directory | DiskFile>();
  async getDirectoryHandle(name: string, options?: { create?: boolean }): Promise<Directory> {
    if (!this.children.has(name) && options?.create) this.children.set(name, new Directory());
    const entry = this.children.get(name);
    if (!(entry instanceof Directory)) throw new DOMException("Missing", "NotFoundError");
    return entry;
  }
  async getFileHandle(name: string, options?: { create?: boolean }): Promise<DiskFile> {
    if (!this.children.has(name) && options?.create) this.children.set(name, new DiskFile(name));
    const entry = this.children.get(name);
    if (!(entry instanceof DiskFile)) throw new DOMException("Missing", "NotFoundError");
    return entry;
  }
  async removeEntry(name: string) {
    this.children.delete(name);
  }
  async *entries() {
    yield* this.children.entries();
  }
}
let diskFull = false;
class DiskFile {
  kind = "file";
  bytes: Blob = new Blob([]);
  writes = 0;
  constructor(public name: string) {}
  async getFile() {
    return new File([this.bytes], this.name);
  }
  async createWritable() {
    this.writes++;
    let next = this.bytes;
    return {
      write: async (data: string | ArrayBuffer | { data: ArrayBuffer; position: number }) => {
        if (diskFull && this.name.endsWith(".part"))
          throw new DOMException("Full", "QuotaExceededError");
        if (typeof data === "object" && "position" in data)
          next = new Blob([next.slice(0, data.position), data.data]);
        else next = new Blob([data as string | ArrayBuffer]);
      },
      truncate: async (size: number) => {
        next = next.slice(0, size);
      },
      close: async () => {
        this.bytes = next;
      },
      abort: async () => {},
    };
  }
}
let root: Directory, target: DiskFile;
beforeEach(() => {
  root = new Directory();
  target = new DiskFile("target");
  target.bytes = new Blob(["original"]);
  diskFull = false;
  vi.stubGlobal("Blob", Blob);
  vi.stubGlobal("navigator", {
    storage: { getDirectory: async () => root },
    locks: {
      request: async (_name: string, _options: unknown, action: () => Promise<unknown>) => action(),
    },
  });
  Object.defineProperty(window, "showSaveFilePicker", {
    configurable: true,
    value: async () => target,
  });
});

async function server(payload: string, checksum?: string) {
  const hash = checksum ?? (await sha256(payload));
  const fetcher = vi.fn(async (_url: unknown, init: RequestInit = {}) => {
    const headers: Record<string, string> = {
      "X-File-Size": String(payload.length),
      "X-File-SHA256": hash,
      ETag: '"version"',
    };
    if (init.method === "HEAD") return new Response(null, { headers });
    headers["Content-Range"] = `bytes 0-${payload.length - 1}/${payload.length}`;
    return new Response(payload, { status: 206, headers });
  });
  vi.stubGlobal("fetch", fetcher);
  return fetcher;
}

it("verifies staging before opening the destination and cleans up success", async () => {
  await server("payload");
  await downloadVerified("file", "file");
  expect(await target.bytes.text()).toBe("payload");
  expect(await stagedDownloads()).toEqual([]);
});

it("preserves the original on checksum failure and allows explicit staging cleanup", async () => {
  await server("payload", "0".repeat(64));
  await expect(downloadVerified("file", "file")).rejects.toThrow("SHA-256");
  expect(target.writes).toBe(0);
  expect(await target.bytes.text()).toBe("original");
  const records = await stagedDownloads();
  expect(records[0].bytes).toBe(7);
  await clearDownload(records[0].id);
  expect(await stagedDownloads()).toEqual([]);
  expect(await target.bytes.text()).toBe("original");
});

it("resumes committed chunks after interruption and rechecks them", async () => {
  const fetcher = await server("payload");
  const controller = new AbortController();
  await expect(
    downloadVerified("file", "file", () => controller.abort(), controller.signal),
  ).rejects.toThrow();
  expect(target.writes).toBe(0);
  expect((await stagedDownloads())[0].bytes).toBe(7);
  fetcher.mockClear();
  await downloadVerified("file", "file");
  expect(fetcher).toHaveBeenCalledTimes(1); // HEAD only; the durable range was reused.
  expect(await target.bytes.text()).toBe("payload");
});

it("redownloads a missing staging chunk without touching the original early", async () => {
  const fetcher = await server("payload");
  const controller = new AbortController();
  await expect(
    downloadVerified("file", "file", () => controller.abort(), controller.signal),
  ).rejects.toThrow();
  const record = (await stagedDownloads())[0];
  await (await (await downloadRoot()).getDirectoryHandle(record.id)).removeEntry("0.part");
  fetcher.mockClear();
  await downloadVerified("file", "file");
  expect(fetcher).toHaveBeenCalledTimes(2);
});

it("disk full leaves the original untouched and staging deletable", async () => {
  await server("payload");
  diskFull = true;
  await expect(downloadVerified("file", "file")).rejects.toThrow("Full");
  expect(target.writes).toBe(0);
  const records = await stagedDownloads();
  await clearDownload(records[0].id);
  expect(await stagedDownloads()).toEqual([]);
});

it("cleanup cancels and waits for active requests before deleting staging", async () => {
  const fetcher = await server("payload");
  const original = fetcher.getMockImplementation()!;
  fetcher.mockImplementation(async (url: unknown, init: RequestInit = {}) => {
    if (init.method === "HEAD") return original(url, init);
    return new Promise<Response>((_resolve, reject) => {
      init.signal?.addEventListener(
        "abort",
        () => reject(new DOMException("Canceled", "AbortError")),
        { once: true },
      );
    });
  });
  const pending = downloadVerified("file", "file");
  const result = pending.catch(() => false);
  await vi.waitFor(async () => expect((await stagedDownloads()).length).toBe(1));
  await clearDownload((await stagedDownloads())[0].id);
  expect(await result).toBe(false);
  expect(await stagedDownloads()).toEqual([]);
  expect(target.writes).toBe(0);
});

it("rechecks staged chunks and redownloads corrupted bytes", async () => {
  const fetcher = await server("payload");
  const controller = new AbortController();
  await expect(
    downloadVerified("file", "file", () => controller.abort(), controller.signal),
  ).rejects.toThrow();
  const record = (await stagedDownloads())[0];
  const directory = await (await downloadRoot()).getDirectoryHandle(record.id);
  const file = await directory.getFileHandle("0.part");
  const writer = await file.createWritable();
  await writer.write("damaged");
  await writer.close();
  fetcher.mockClear();
  await downloadVerified("file", "file");
  expect(fetcher).toHaveBeenCalledTimes(2);
  expect(await target.bytes.text()).toBe("payload");
});
