import { useState } from "react";
import { File, Pause, Play, Trash2, X } from "lucide-react";
import { Button } from "../../components/ui/button";
import { Card } from "../../components/ui/card";
import { usePreferences } from "../../lib/preferences";
import { formatBytes } from "../../lib/utils";
import { useUploadQueue } from "./use-upload-queue";

export function UploadTasks() {
  const { t } = usePreferences();
  const queue = useUploadQueue();
  const [error, setError] = useState(false);
  return (
    <Card className="space-y-5" aria-label={t("queue")}>
      <div className="flex items-center justify-between gap-2">
        <h3 className="font-medium">
          {t("queue")} <span className="text-xs text-muted-foreground">{queue.items.length}</span>
        </h3>
        <div className="flex items-center gap-2">
          <label htmlFor="upload-concurrency" className="text-xs text-muted-foreground">
            {t("concurrency")}
          </label>
          <select
            id="upload-concurrency"
            aria-label={t("concurrency")}
            className="rounded-md border border-border bg-background px-2 py-1 text-sm"
            value={queue.concurrency}
            disabled={queue.running}
            onChange={(event) => queue.setConcurrency(Number(event.target.value))}
          >
            {[1, 2, 3, 4].map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </select>
          <Button
            variant="ghost"
            id="clear-queue"
            disabled={queue.running || !queue.items.some((item) => item.phase === "completed")}
            onClick={queue.clear}
          >
            <Trash2 aria-hidden="true" />
            {t("clear")}
          </Button>
        </div>
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {t("cancelFailed")}
        </p>
      )}
      <div id="queue-list" className="max-h-[32rem] space-y-3 overflow-y-auto" aria-live="polite">
        {!queue.items.length && (
          <p className="py-4 text-center text-sm text-muted-foreground">{t("noQueue")}</p>
        )}
        {queue.items.map((item) => {
          const percentage =
            item.file.size === 0
              ? item.phase === "completed"
                ? 100
                : 0
              : Math.min(100, Math.floor((item.offset / item.file.size) * 100));
          return (
            <article key={item.id} className="queue-item rounded-xl border border-border p-3">
              <div className="flex items-start gap-2">
                <File className="mt-1 size-4 shrink-0 text-primary" aria-hidden="true" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium" title={item.file.name}>
                    {item.file.name}
                  </p>
                  <span className={`queue-state state-${item.phase} text-xs text-muted-foreground`}>
                    {t(item.phase)}
                  </span>
                </div>
                <Button
                  variant="ghost"
                  className="px-2"
                  aria-label={`${t(item.phase === "completed" ? "remove" : "cancel")} ${item.file.name}`}
                  onClick={() => {
                    setError(false);
                    void queue.cancel(item).catch(() => setError(true));
                  }}
                >
                  <X aria-hidden="true" />
                </Button>
              </div>
              <progress
                className="my-2 h-1.5 w-full accent-primary"
                aria-label={`${item.file.name} ${t("status")}`}
                max={100}
                value={percentage}
              />
              <p className="text-xs text-muted-foreground">
                {percentage}% · {formatBytes(item.offset)} / {formatBytes(item.file.size)}
              </p>
              {item.phase === "uploading" && item.speed > 0 && (
                <p className="mt-1 text-xs text-muted-foreground">
                  {formatBytes(item.speed)}/s · {t("remaining")}{" "}
                  {Math.ceil((item.file.size - item.offset) / item.speed)} {t("second")}
                </p>
              )}
              {item.error && (
                <p className="mt-2 text-xs text-destructive">
                  {item.errorCode === "destination_changed" ? t("overwriteChanged") : t("error")}
                </p>
              )}
            </article>
          );
        })}
      </div>
      <Button
        id="upload"
        className="w-full"
        disabled={queue.running || !queue.items.some((item) => item.phase !== "completed")}
        onClick={() => void queue.start()}
      >
        <Play aria-hidden="true" />
        {t(
          queue.items.some((item) => ["paused", "failed"].includes(item.phase))
            ? "resume"
            : "start",
        )}
      </Button>
      <Button
        id="stop-upload"
        variant="outline"
        className="w-full"
        hidden={!queue.running}
        onClick={queue.pause}
      >
        <Pause aria-hidden="true" />
        {t("pause")}
      </Button>
    </Card>
  );
}
