import { expect, it, vi } from "vitest";
import { clientID } from "./utils";
it("creates protocol-compatible keys without secure-context randomUUID", () => {
  vi.stubGlobal("crypto", { getRandomValues: (bytes: Uint8Array) => bytes.fill(171) });
  expect(clientID()).toBe("ab".repeat(16));
});
