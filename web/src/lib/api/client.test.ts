import { expect, it } from "vitest";
import { apiErrorFromResponse } from "./client";

it("parses API error message, code, and retry delay consistently", async () => {
  const response = new Response(JSON.stringify({ error: "destination already exists", code: "destination_exists" }), {
    status: 409,
    headers: { "Retry-After": "2" },
  });

  await expect(apiErrorFromResponse(response)).resolves.toMatchObject({
    status: 409,
    message: "destination already exists",
    code: "destination_exists",
    retryAfterMS: 2000,
  });
});

it("falls back to the HTTP status for malformed error responses", async () => {
  const error = await apiErrorFromResponse(new Response("not json", { status: 412 }));

  expect(error).toMatchObject({ status: 412, message: "HTTP 412", code: undefined });
});
