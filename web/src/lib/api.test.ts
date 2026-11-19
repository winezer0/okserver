import { expect, it, vi } from "vitest";
import { APIError, apiURL, cancelUpload, directorySchema, retryQuery } from "./api";
it("rejects malformed directory contracts and retries only recoverable failures", () => {
  expect(directorySchema.safeParse({ entries: "broken" }).success).toBe(false);
  expect(retryQuery(0, new APIError(401, "auth"))).toBe(false);
  expect(retryQuery(0, new APIError(429, "busy"))).toBe(true);
  expect(retryQuery(2, new APIError(500, "error"))).toBe(false);
  expect(apiURL("/api/v1/files?path=test")).toBe("/api/v1/files?path=test");
});
it("retries temporary cancellation locks and accepts already canceled tasks", async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(new Response('{"error":"busy"}', { status: 423 }))
    .mockResolvedValueOnce(new Response('{"uploads":[]}'))
    .mockResolvedValueOnce(new Response(null, { status: 204 }))
    .mockResolvedValueOnce(new Response('{"error":"missing"}', { status: 404 }));
  vi.stubGlobal("fetch", fetchMock);
  await cancelUpload("task");
  expect(fetchMock).toHaveBeenCalledTimes(3);
  await cancelUpload("task");
  vi.unstubAllGlobals();
});
