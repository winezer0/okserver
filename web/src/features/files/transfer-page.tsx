import { useEffect, useState } from "react";
import { Link, useLocation } from "react-router";
import { useUploadQueue } from "../uploads/use-upload-queue";
import { useTaskEvents } from "../uploads/use-task-events";
import { usePreferences } from "../../lib/preferences";
import { Button } from "../../components/ui/button";
import { Card } from "../../components/ui/card";
import { cancelUpload } from "../../lib/api";
import { formatBytes } from "../../lib/utils";
import { stagedDownloads } from "./download-store";
import { clearDownload, downloadVerified, isDownloadActive, pauseDownload } from "./download";
import { downloadHistory, removeDownloadHistory } from "./transfer-records";

interface Task {
  id: string;
  path: string;
  size: number;
  offset: number;
  type: "upload" | "download";
  state: "active" | "paused" | "completed" | "failed";
  detail?: string;
  action?: () => Promise<unknown> | void;
  remove?: () => Promise<unknown> | void;
}

export function TransferPage() {
  const queue = useUploadQueue();
  const { live, query } = useTaskEvents();
  const { locale, t } = usePreferences();
  const zh = locale === "zh";
  const { search } = useLocation();
  const [kind, setKind] = useState("all");
  const [state, setState] = useState("all");
  const [error, setError] = useState("");
  const [staged, setStaged] = useState<Awaited<ReturnType<typeof stagedDownloads>>>([]);
  const [, tick] = useState(0);
  useEffect(() => {
    let mounted = true;
    const refresh = async () => {
      try {
        const records =
          typeof navigator.storage?.getDirectory === "function" ? await stagedDownloads() : [];
        if (mounted) setStaged(records);
      } catch (e) {
        if (mounted) setError(String(e));
      }
      if (mounted) tick((n) => n + 1);
    };
    void refresh();
    const timer = setInterval(() => void refresh(), 1000);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, []);
  const run = async (action?: Task["action"]) => {
    setError("");
    try {
      await action?.();
      tick((n) => n + 1);
      void query.refetch();
    } catch (e) {
      setError(String(e));
    }
  };
  const tasks: Task[] = queue.items.map((item) => ({
    id: item.id,
    path: [item.directory, item.file.name].filter(Boolean).join("/"),
    size: item.file.size,
    offset: item.offset,
    type: "upload",
    state:
      item.phase === "completed"
        ? "completed"
        : item.phase === "failed"
          ? "failed"
          : item.phase === "paused" || item.phase === "waiting"
            ? "paused"
            : "active",
    detail: item.error || (item.speed ? `${formatBytes(item.speed)}/s` : undefined),
    action: () =>
      ["uploading", "hashing", "retrying", "verifying"].includes(item.phase)
        ? queue.pause()
        : queue.start(),
    remove: () => (item.phase === "completed" ? queue.remove(item.id) : queue.cancel(item)),
  }));
  for (const remote of query.data?.uploads || []) {
    if (
      queue.items.some(
        (item) =>
          item.sessionId === remote.id ||
          [item.directory, item.file.name].filter(Boolean).join("/") === remote.path,
      )
    )
      continue;
    tasks.push({
      id: remote.id,
      path: remote.path,
      size: remote.size,
      offset: remote.offset,
      type: "upload",
      state: "active",
      remove: () => cancelUpload(remote.id),
    });
  }
  for (const record of staged)
    tasks.push({
      id: record.id,
      path: record.path,
      size: record.size,
      offset: record.bytes,
      type: "download",
      state: isDownloadActive(record.id)
        ? "active"
        : downloadHistory().find((r) => r.id === record.id)?.error
          ? "failed"
          : "paused",
      detail: downloadHistory().find((r) => r.id === record.id)?.error,
      action: () =>
        isDownloadActive(record.id)
          ? pauseDownload(record.id)
          : downloadVerified(record.path, record.name),
      remove: async () => {
        await clearDownload(record.id);
        removeDownloadHistory(record.id);
        setStaged((rows) => rows.filter((row) => row.id !== record.id));
      },
    });
  for (const record of downloadHistory())
    if (record.completed && !staged.some((r) => r.id === record.id))
      tasks.push({
        id: record.id,
        path: record.path,
        size: record.size,
        offset: record.size,
        type: "download",
        state: "completed",
        remove: () => {
          removeDownloadHistory(record.id);
        },
      });
  const filtered = tasks.filter(
    (task) => (kind === "all" || task.type === kind) && (state === "all" || task.state === state),
  );
  return (
    <section className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex gap-2">
          {[
            ["all", zh ? "全部" : "All"],
            ["upload", zh ? "上传" : "Uploads"],
            ["download", zh ? "下载" : "Downloads"],
          ].map(([key, label]) => (
            <Button
              key={key}
              variant={kind === key ? "default" : "ghost"}
              aria-pressed={kind === key}
              onClick={() => setKind(key)}
            >
              {label} {tasks.filter((task) => key === "all" || task.type === key).length}
            </Button>
          ))}
        </div>
        <span data-testid="event-status" className="text-sm text-muted-foreground">
          {t(live ? "connection" : "polling")}
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <select
          className="font-sans text-sm leading-5 text-foreground"
          aria-label={zh ? "任务状态" : "Task status"}
          value={state}
          onChange={(event) => setState(event.target.value)}
        >
          {[
            ["all", zh ? "全部状态" : "All states"],
            ["active", zh ? "进行中" : "Active"],
            ["paused", t("paused")],
            ["completed", t("completed")],
            ["failed", t("failed")],
          ].map(([key, label]) => (
            <option key={key} value={key}>
              {label}
            </option>
          ))}
        </select>
        <Button id="refresh-tasks" variant="outline" onClick={() => void query.refetch()}>
          {t("refresh")}
        </Button>
        <Button
          variant="outline"
          disabled={!tasks.some((task) => task.state === "completed")}
          onClick={() =>
            void run(async () => {
              for (const task of tasks.filter((row) => row.state === "completed"))
                await task.remove?.();
            })
          }
        >
          {zh ? "清理已完成" : "Clear completed"}
        </Button>
      </div>
      {(error || query.isError) && <p role="alert">{error || t("requestFailed")}</p>}
      <Card className="divide-y divide-border">
        {!filtered.length && (
          <div className="space-y-3 py-12 text-center">
            <p>{zh ? "暂无传输任务" : "No transfer tasks"}</p>
            <p className="text-sm text-muted-foreground">
              {zh ? "上传和下载的任务将在这里显示。" : "Uploads and downloads appear here."}
            </p>
            <Link className="nav-link inline-block" to={`/files${search}`}>
              {zh ? "前往文件浏览" : "Browse files"}
            </Link>
          </div>
        )}
        {filtered.map((task) => (
          <article key={`${task.type}:${task.id}`} className="space-y-3 py-5">
            <div className="flex flex-wrap justify-between gap-3">
              <strong className="min-w-0 break-all">
                {task.type === "upload" ? "↑" : "↓"} {task.path}
              </strong>
              <span
                className={`inline-flex items-center rounded-md px-2.5 py-1 text-sm font-medium ${
                  task.state === "failed"
                    ? "bg-destructive/10 text-destructive"
                    : task.state === "completed"
                      ? "bg-primary/10 text-primary"
                      : task.state === "active"
                        ? "bg-primary/10 text-primary"
                        : "bg-muted text-muted-foreground"
                }`}
              >
                {task.state === "active" ? (zh ? "进行中" : "Active") : t(task.state)}
              </span>
            </div>
            {task.state !== "completed" && (
              <progress
                className="h-2 w-full accent-primary"
                aria-label={task.path}
                max={Math.max(1, task.size)}
                value={task.offset}
              />
            )}
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-sm text-muted-foreground">
                {formatBytes(task.offset)} / {formatBytes(task.size)}
                {task.state === "completed"
                  ? " · SHA-256 ✓"
                  : task.detail
                    ? ` · ${task.detail}`
                    : ""}
              </p>
              <div className="flex gap-2">
                {task.action && task.state !== "completed" && (
                  <Button variant="outline" onClick={() => void run(task.action)}>
                    {task.state === "active" ? t("pause") : t("resume")}
                  </Button>
                )}
                <Button variant="ghost" onClick={() => void run(task.remove)}>
                  {task.state === "completed"
                    ? t("remove")
                    : zh
                      ? "取消并清理"
                      : "Cancel and delete"}
                </Button>
              </div>
            </div>
          </article>
        ))}
      </Card>
    </section>
  );
}
