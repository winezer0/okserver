import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { cancelUpload } from "../../lib/api";
import { APIError } from "../../lib/api";
import { clientID } from "../../lib/utils";
import { usePreferences } from "../../lib/preferences";
import type { Progress } from "./transfer";
import { serialUpload as uploadFile } from "./serial-transfer";

export interface QueueItem extends Progress {
  id: string;
  file: File;
  directory: string;
  speed: number;
  error?: string;
  errorCode?: string;
}
interface QueueContext {
  items: QueueItem[];
  running: boolean;
  concurrency: number;
  setConcurrency: (value: number) => void;
  add: (files: FileList | File[], directory: string) => void;
  start: () => Promise<void>;
  pause: () => void;
  remove: (id: string) => void;
  clear: () => void;
  cancel: (item: QueueItem) => Promise<void>;
}
const Context = createContext<QueueContext | null>(null);
const concurrencyKey = "okserver:upload-concurrency";
function initialConcurrency() {
  try {
    const value = Number(localStorage.getItem(concurrencyKey));
    return Number.isInteger(value) && value >= 1 && value <= 4 ? value : 2;
  } catch {
    return 2;
  }
}
export function UploadQueueProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<QueueItem[]>([]);
  const [running, setRunning] = useState(false);
  const [concurrency, setConcurrencyState] = useState(initialConcurrency);
  const state = useRef(items);
  const controller = useRef<AbortController | null>(null);
  const itemControllers = useRef(new Map<string, AbortController>());
  const cancelIDs = useRef(new Set<string>());
  const client = useQueryClient();
  const { t } = usePreferences();
  const update = (transform: (items: QueueItem[]) => QueueItem[]) => {
    state.current = transform(state.current);
    setItems(state.current);
  };
  useEffect(() => () => controller.current?.abort(), []);
  function setConcurrency(value: number) {
    const bounded = Number.isFinite(value) ? Math.max(1, Math.min(4, Math.floor(value))) : 1;
    setConcurrencyState(bounded);
    try {
      localStorage.setItem(concurrencyKey, String(bounded));
    } catch {
      /* The preference is optional. */
    }
  }
  function add(files: FileList | File[], directory: string) {
    update((current) => {
      const identities = new Set(
        current.map(
          (item) =>
            `${item.directory}\0${item.file.name}\0${item.file.size}\0${item.file.lastModified}`,
        ),
      );
      return [
        ...current,
        ...Array.from(files)
          .filter((file) => {
            const identity = `${directory}\0${file.name}\0${file.size}\0${file.lastModified}`;
            if (identities.has(identity)) return false;
            identities.add(identity);
            return true;
          })
          .map((file) => ({
            id: clientID(),
            file,
            directory,
            phase: "waiting" as const,
            offset: 0,
            speed: 0,
          })),
      ];
    });
  }
  async function start() {
    if (controller.current) return;
    const active = new AbortController();
    controller.current = active;
    setRunning(true);
    const pending = [...state.current].filter((item) => item.phase !== "completed");
    let cursor = 0;
    const runWorker = async () => {
      while (!active.signal.aborted) {
        const item = pending[cursor++];
        if (!item) return;
        if (!state.current.some((candidate) => candidate.id === item.id)) continue;
        const itemController = new AbortController();
        itemControllers.current.set(item.id, itemController);
        const pauseItem = () => itemController.abort();
        active.signal.addEventListener("abort", pauseItem, { once: true });
        let lastTime = performance.now();
        let lastOffset = 0;
        try {
          await uploadFile(
            item.file,
            item.directory,
            (progress) => {
              const now = performance.now();
              const speed =
                progress.phase === "uploading" && now > lastTime && progress.offset >= lastOffset
                  ? ((progress.offset - lastOffset) * 1000) / (now - lastTime)
                  : 0;
              lastTime = now;
              lastOffset = progress.offset;
              update((current) =>
                current.map((candidate) =>
                  candidate.id === item.id
                    ? {
                        ...candidate,
                        ...progress,
                        sessionId: progress.sessionId || candidate.sessionId,
                        speed,
                        error: undefined,
                      }
                    : candidate,
                ),
              );
            },
            itemController.signal,
            (path) => window.confirm(`${t("overwriteConfirm")}\n\n${path}`),
          );
        } catch (error) {
          const canceled = error instanceof Error && error.name === "AbortError";
          update((current) =>
            current.map((candidate) =>
              candidate.id === item.id
                ? {
                    ...candidate,
                    phase: itemController.signal.aborted || canceled ? "paused" : "failed",
                    error: itemController.signal.aborted || canceled ? undefined : String(error),
                    errorCode: error instanceof APIError ? error.code : undefined,
                  }
                : candidate,
            ),
          );
        }
        if (cancelIDs.current.delete(item.id)) {
          const target = state.current.find((candidate) => candidate.id === item.id);
          try {
            if (target?.sessionId) await cancelUpload(target.sessionId);
            update((current) => current.filter((candidate) => candidate.id !== item.id));
          } catch (error) {
            update((current) =>
              current.map((candidate) =>
                candidate.id === item.id
                  ? { ...candidate, phase: "failed", error: String(error) }
                  : candidate,
              ),
            );
          }
        }
        active.signal.removeEventListener("abort", pauseItem);
        itemControllers.current.delete(item.id);
        void client.invalidateQueries({ queryKey: ["files"] });
        void client.invalidateQueries({ queryKey: ["tasks"] });
      }
    };
    try {
      await Promise.all(Array.from({ length: Math.min(concurrency, pending.length) }, runWorker));
    } finally {
      for (const upload of itemControllers.current.values()) upload.abort();
      itemControllers.current.clear();
      controller.current = null;
      setRunning(false);
    }
  }
  async function cancel(item: QueueItem) {
    const itemController = itemControllers.current.get(item.id);
    if (itemController) {
      cancelIDs.current.add(item.id);
      itemController.abort();
      return;
    }
    if (item.sessionId && item.phase !== "completed") await cancelUpload(item.sessionId);
    update((current) => current.filter((candidate) => candidate.id !== item.id));
    void client.invalidateQueries({ queryKey: ["tasks"] });
  }
  return (
    <Context.Provider
      value={{
        items,
        running,
        concurrency,
        setConcurrency,
        add,
        start,
        pause: () => controller.current?.abort(),
        cancel,
        remove: (id) => update((current) => current.filter((item) => item.id !== id)),
        clear: () => update((current) => current.filter((item) => item.phase !== "completed")),
      }}
    >
      {children}
    </Context.Provider>
  );
}
export function useUploadQueue() {
  const value = useContext(Context);
  if (!value) throw new Error("Missing queue provider");
  return value;
}
