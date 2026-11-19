import { describe, expect, it } from "vitest";
import { z } from "zod";
import { APIError, isRetryable, retryQuery, retryAfterMS } from "./client";

describe("request retry policy", () => {
  it("reads Retry-After seconds and dates", () => {
    expect(retryAfterMS(new Response(null, { headers: { "Retry-After": "3" } }))).toBe(3000);
    expect(retryAfterMS(new Response(null, { headers: { "Retry-After": "invalid" } }))).toBe(0);
    expect(
      retryAfterMS(
        new Response(null, {
          headers: { "Retry-After": new Date(Date.now() + 60000).toUTCString() },
        }),
      ),
    ).toBeGreaterThan(58000);
  });
  it("separates transient failures from permanent errors", () => {
    for (const status of [408, 423, 429, 500, 503]) {
      expect(isRetryable(new APIError(status, "temporary"))).toBe(true);
    }
    for (const status of [400, 401, 403, 404, 413, 422]) {
      expect(isRetryable(new APIError(status, "permanent"))).toBe(false);
    }
    expect(isRetryable(new APIError(409, "offset"))).toBe(false);
    expect(isRetryable(new APIError(409, "offset"), true)).toBe(true);
  });

  it("does not retry cancellation or invalid response contracts", () => {
    expect(isRetryable(new DOMException("paused", "AbortError"))).toBe(false);
    const invalid = z.object({ offset: z.number() }).safeParse({ offset: "wrong" });
    expect(invalid.success).toBe(false);
    if (!invalid.success) expect(isRetryable(invalid.error)).toBe(false);
    expect(retryQuery(2, new TypeError("network"))).toBe(false);
  });
});
