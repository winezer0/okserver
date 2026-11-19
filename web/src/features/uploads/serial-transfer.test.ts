import { afterEach, expect, it, vi } from "vitest";
import { File } from "node:buffer";
import { sha256 } from "hash-wasm";
import { serialUpload } from "./serial-transfer";

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

for (const mode of [
  "normal",
  "lost-chunk",
  "lost-final",
  "async",
  "resume",
  "bad-prefix",
  "bad-final",
  "empty",
  "failed",
  "canceled",
] as const) {
  it(`serial protocol: ${mode}`, async () => {
    const payload = mode === "empty" ? "" : "abcdefghijk";
    const file = new File([payload], "file.bin") as unknown as globalThis.File;
    const checksum = await sha256(payload);
    let stored = mode === "resume" || mode === "bad-prefix" ? payload.slice(0, 4) : "";
    let created = stored.length > 0;
    let completed = false;
    let dropped = false;
    let polls = 0;
    let queried = false;
    const progress = vi.fn();
    const fetchMock = vi.fn(async (_url: unknown, options: RequestInit = {}) => {
      const headers = () => ({
        "Upload-Offset": String(stored.length),
        "Upload-Length": String(file.size),
        "Upload-Chunk-Size": "4",
        "Upload-ID": "task",
        "Upload-State": completed ? "completed" : "uploading",
        "Upload-Prefix-SHA256": "",
        "X-File-SHA256": completed ? checksum : "",
      });
      if (options.method === "HEAD") {
        if (!queried) {
          queried = true;
          if (mode === "failed" || mode === "canceled")
            return new Response(null, {
              headers: { "Upload-State": mode, "Upload-Length": "999", "Upload-Chunk-Size": "4" },
            });
        }
        if (!created)
          return new Response(null, { status: 404, headers: { "Upload-Chunk-Size": "4" } });
        if (mode === "async" && stored.length === file.size && ++polls > 1) completed = true;
        const values = headers();
        values["Upload-Prefix-SHA256"] =
          mode === "bad-prefix" ? "0".repeat(64) : await sha256(stored);
        return new Response(null, { headers: values });
      }
      const bytes = new Uint8Array(options.body as ArrayBuffer);
      const part = new TextDecoder().decode(bytes);
      const sent = new Headers(options.headers);
      expect(sent.get("Upload-Token")).toMatch(/^[a-f0-9]{32}$/);
      if (file.size) {
        expect(sent.get("X-Chunk-SHA256")).toBe(await sha256(bytes));
        expect(bytes.length).toBeLessThanOrEqual(4);
      }
      stored += part;
      created = true;
      const final = stored.length === file.size;
      if (final) {
        expect(sent.get("X-File-SHA256")).toBe(checksum);
        completed = mode !== "async";
      }
      if (!dropped && ((mode === "lost-chunk" && !final) || (mode === "lost-final" && final))) {
        dropped = true;
        throw new TypeError("lost acknowledgement");
      }
      const values = headers();
      if (mode === "bad-final" && final) values["X-File-SHA256"] = "0".repeat(64);
      return new Response(null, {
        status: final ? (mode === "async" ? 202 : 201) : 204,
        headers: values,
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const run = serialUpload(file, "nested", progress, new AbortController().signal);
    if (mode === "bad-prefix") await expect(run).rejects.toThrow("SESSION_MISMATCH");
    else if (mode === "bad-final") await expect(run).rejects.toThrow("INTEGRITY_CHECK_FAILED");
    else {
      await run;
      expect(stored).toBe(payload);
      expect(progress).toHaveBeenLastCalledWith(
        expect.objectContaining({ phase: "completed", offset: file.size }),
      );
    }
    expect(
      fetchMock.mock.calls.every(([url]) => String(url).includes("/upload/nested/file.bin")),
    ).toBe(true);
  });
}
