import { Folder } from "lucide-react";
import { useState } from "react";
import { Button } from "../../components/ui/button";
import { Card } from "../../components/ui/card";
import { usePreferences } from "../../lib/preferences";
import { formatBytes } from "../../lib/utils";
import { useUploadQueue } from "../uploads/use-upload-queue";
import { useDirectory } from "./use-directory";
import { FileTable } from "./file-table";
import { UploadDialog } from "../uploads/upload-dialog";
import { DirectoryToolbar, DirectoryPagination } from "./directory-toolbar";

export function DirectoryPage() {
  const { t } = usePreferences();
  const model = useDirectory();
  const { directory, query, search } = model;
  const [open, setOpen] = useState(false);
  const queue = useUploadQueue();
  return (
    <>
      <section
        className="mb-4 flex flex-wrap items-center gap-4 text-sm text-muted-foreground"
        aria-label={t("workspace")}
      >
        {[
          { label: t("files"), value: query.data?.files || 0 },
          { label: t("folders"), value: query.data?.folders || 0 },
          { label: t("totalSize"), value: formatBytes(query.data?.bytes || 0) },
        ].map(({ label, value }) => (
          <span key={label}>
            {label} <strong className="ml-1 text-foreground tabular-nums">{value}</strong>
          </span>
        ))}
      </section>
      <div
        onDragOver={(event) => {
          if (event.dataTransfer.types.includes("Files")) event.preventDefault();
        }}
        onDrop={(event) => {
          if (!event.dataTransfer.files.length) return;
          event.preventDefault();
          queue.add(event.dataTransfer.files, directory);
          setOpen(true);
        }}
      >
        <Card className="min-w-0 space-y-5" aria-labelledby="files-heading">
          <DirectoryToolbar
            model={model}
            onUpload={() => {
              setOpen(true);
              requestAnimationFrame(() => document.getElementById("choose-files")?.focus());
            }}
          />
          {query.isError && (
            <div role="alert" className="rounded-lg bg-destructive/10 p-4 text-sm text-destructive">
              {t("requestFailed")}{" "}
              <Button variant="ghost" onClick={() => void query.refetch()}>
                {t("retry")}
              </Button>
            </div>
          )}
          <FileTable
            entries={query.data?.entries || []}
            total={query.data?.total || 0}
            directoryLink={model.directoryLink}
          />
          {query.isPending && (
            <p role="status" className="py-10 text-center text-sm text-muted-foreground">
              {t("loading")}
            </p>
          )}
          {!query.isPending && !query.isError && query.data?.entries.length === 0 && (
            <div className="py-12 text-center">
              <Folder className="mx-auto mb-3 size-10 text-muted-foreground" aria-hidden="true" />
              <h3 className="font-medium">{t(search ? "noResults" : "empty")}</h3>
              <p className="mt-2 text-sm text-muted-foreground">{t("emptyHint")}</p>
            </div>
          )}
          <DirectoryPagination model={model} />
        </Card>
        <UploadDialog directory={directory} open={open} onClose={() => setOpen(false)} />
      </div>
    </>
  );
}
