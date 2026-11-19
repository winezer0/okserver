import { expect, it, vi } from "vitest";
import { retryFetch } from "./retry-fetch";
import { delay } from "../../features/uploads/transfer";
vi.mock("../../features/uploads/transfer", () => ({ delay: vi.fn(async () => {}) }));

it("retries transient status with Retry-After and bounded attempts", async () => {
  vi.mocked(delay).mockClear();
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(new Response(null, { status: 429, headers: { "Retry-After": "2" } }))
    .mockResolvedValueOnce(new Response(null, { status: 503 }))
    .mockResolvedValue(new Response(null));
  vi.stubGlobal("fetch", fetcher);
  expect((await retryFetch("/file", { method: "HEAD" })).status).toBe(200);
  expect(fetcher).toHaveBeenCalledTimes(3);
  expect(vi.mocked(delay).mock.calls[0][0]).toBeGreaterThanOrEqual(2000);
  fetcher.mockReset().mockResolvedValue(new Response(null, { status: 503 }));
  await expect(retryFetch("/file", {})).rejects.toThrow("503");
  expect(fetcher).toHaveBeenCalledTimes(6);
});

it("does not retry authorization or canceled requests", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 401 }));
  vi.stubGlobal("fetch", fetcher);
  expect((await retryFetch("/file", {})).status).toBe(401);
  expect(fetcher).toHaveBeenCalledTimes(1);
  const controller = new AbortController();
  controller.abort();
  await expect(retryFetch("/file", { signal: controller.signal })).rejects.toThrow();
  expect(fetcher).toHaveBeenCalledTimes(1);
});
