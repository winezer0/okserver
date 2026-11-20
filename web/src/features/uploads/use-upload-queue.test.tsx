import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";
import { UploadQueueProvider, useUploadQueue } from "./use-upload-queue";
import { serialUpload as uploadFile } from "./serial-transfer";
import { cancelUpload } from "../../lib/api";
import { afterEach } from "vitest";
import { Preferences } from "../../lib/preferences";

vi.mock("./serial-transfer", () => ({ serialUpload: vi.fn() }));
vi.mock("../../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../lib/api")>()),
  cancelUpload: vi.fn().mockResolvedValue(undefined),
}));
afterEach(() => {
  localStorage.clear();
});

it("cancels one active upload, then continues with the queued file", async () => {
  localStorage.setItem("okserver:upload-concurrency", "1");
  vi.mocked(uploadFile).mockImplementation(async (file, _directory, progress, signal) => {
    if (file.name === "second.bin") {
      progress({ offset: file.size, phase: "completed" });
      return;
    }
    progress({ offset: 4, phase: "uploading", sessionId: "session" });
    await new Promise<void>((_resolve, reject) =>
      signal.addEventListener("abort", () => reject(new DOMException("Paused", "AbortError")), {
        once: true,
      }),
    );
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <Preferences>
      <QueryClientProvider client={client}>
        <UploadQueueProvider>{children}</UploadQueueProvider>
      </QueryClientProvider>
    </Preferences>
  );
  const { result } = renderHook(useUploadQueue, { wrapper });
  act(() =>
    result.current.add(
      [new File(["first"], "first.bin"), new File(["second"], "second.bin")],
      "nested",
    ),
  );
  let completed: Promise<void>;
  act(() => {
    completed = result.current.start();
  });
  await waitFor(() => expect(result.current.items[0].sessionId).toBe("session"));
  await act(async () => {
    await result.current.cancel(result.current.items[0]);
    await completed;
  });
  expect(cancelUpload).toHaveBeenCalledWith("session");
  expect(result.current.running).toBe(false);
  expect(result.current.items.map((item) => item.file.name)).toEqual(["second.bin"]);
  expect(result.current.items[0].phase).toBe("completed");
  expect(uploadFile).toHaveBeenCalledTimes(2);
});

it("runs the configured number of transfers concurrently and persists the setting", async () => {
  const waiting: Array<() => void> = [];
  vi.mocked(uploadFile).mockImplementation(async (_file, _directory, progress, signal) => {
    progress({ offset: 1, phase: "uploading" });
    await new Promise<void>((resolve, reject) => {
      waiting.push(resolve);
      signal.addEventListener("abort", () => reject(new DOMException("Paused", "AbortError")), {
        once: true,
      });
    });
    progress({ offset: 1, phase: "completed" });
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <Preferences>
      <QueryClientProvider client={client}>
        <UploadQueueProvider>{children}</UploadQueueProvider>
      </QueryClientProvider>
    </Preferences>
  );
  const { result } = renderHook(useUploadQueue, { wrapper });
  act(() => {
    result.current.setConcurrency(2);
    result.current.add(
      [new File(["1"], "one"), new File(["2"], "two"), new File(["3"], "three")],
      "",
    );
  });
  let completed!: Promise<void>;
  act(() => {
    completed = result.current.start();
  });
  await waitFor(() => expect(uploadFile).toHaveBeenCalledTimes(2));
  expect(result.current.concurrency).toBe(2);
  expect(localStorage.getItem("okserver:upload-concurrency")).toBe("2");
  await act(async () => {
    waiting.splice(0).forEach((resolve) => resolve());
    await waitFor(() => expect(uploadFile).toHaveBeenCalledTimes(3));
  });
  await act(async () => {
    waiting.splice(0).forEach((resolve) => resolve());
    await completed;
  });
  expect(result.current.items.every((item) => item.phase === "completed")).toBe(true);
});
