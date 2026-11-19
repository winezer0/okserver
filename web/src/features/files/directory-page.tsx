import { Folder } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "../../components/ui/button";
import { Card } from "../../components/ui/card";
import { usePreferences } from "../../lib/preferences";
import { formatBytes } from "../../lib/utils";
import { useUploadQueue } from "../uploads/use-upload-queue";
import { useDirectory } from "./use-directory";
import { FileTable } from "./file-table";
import { UploadDialog } from "../uploads/upload-dialog";
import { DirectoryToolbar, DirectoryPagination } from "./directory-toolbar";
import { request, capabilitiesSchema } from "../../lib/api";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CreateDirectoryDialog, DeleteConfirmDialog } from "./management-dialogs";
import { apiURL } from "../../lib/api";

export function DirectoryPage() {
  const { t } = usePreferences();
  const model = useDirectory();
  const { directory, query, search } = model;
  const [open, setOpen] = useState(false);
  const queue = useUploadQueue();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [deletePaths, setDeletePaths] = useState<string[] | null>(null);
  const [selectionNotice, setSelectionNotice] = useState("");
  const capabilities = useQuery({
    queryKey: ["capabilities"],
    queryFn: () => request("/api/v1/capabilities", capabilitiesSchema),
  });
  useEffect(() => {
    setSelected(new Set());
  }, [directory, model.page, model.search, model.sort, model.direction]);
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
            allowCreateDir={capabilities.data?.allow_create_dir === true}
            allowDeleteFile={capabilities.data?.allow_delete_file === true}
            onCreateDirectory={() => setCreateOpen(true)}
            selectMode={selectMode}
            onToggleSelectMode={() => {
              setSelectMode((value) => !value);
              setSelected(new Set());
            }}
            selectedCount={selected.size}
            pageCount={query.data?.entries.length || 0}
            onSelectPage={() =>
              setSelected((current) => {
                const pagePaths = query.data?.entries.map((entry) => entry.path) || [];
                const allSelected =
                  pagePaths.length > 0 && pagePaths.every((path) => current.has(path));
                const next = new Set(current);
                pagePaths.forEach((path) => (allSelected ? next.delete(path) : next.add(path)));
                return next;
              })
            }
            onDeleteSelected={() => setDeletePaths([...selected])}
            onDownloadSelected={() => {
              const entries = (query.data?.entries || []).filter((entry) =>
                selected.has(entry.path),
              );
              const files = entries.filter((entry) => !entry.is_dir);
              const skipped = entries.some((entry) => entry.is_dir);
              files.forEach((entry) => {
                const link = document.createElement("a");
                link.href = apiURL(
                  `/download/${entry.path.split("/").map(encodeURIComponent).join("/")}`,
                );
                link.download = entry.name;
                document.body.append(link);
                link.click();
                link.remove();
              });
              setSelectionNotice(skipped ? t("skippedDirectories") : "");
            }}
            onUpload={() => {
              setOpen(true);
              requestAnimationFrame(() => document.getElementById("choose-files")?.focus());
            }}
          />
          {selectionNotice && (
            <p role="status" className="rounded-md bg-muted p-3 text-sm">
              {selectionNotice}
            </p>
          )}
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
            allowDelete={capabilities.data?.allow_delete_file === true}
            selectMode={selectMode}
            selected={selected}
            onToggle={(path) =>
              setSelected((current) => {
                const next = new Set(current);
                next.has(path) ? next.delete(path) : next.add(path);
                return next;
              })
            }
            onDeletePath={(path) => setDeletePaths([path])}
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
        <CreateDirectoryDialog
          directory={directory}
          open={createOpen}
          onClose={() => setCreateOpen(false)}
          refresh={() => void queryClient.invalidateQueries({ queryKey: ["files"] })}
        />
        <DeleteConfirmDialog
          paths={deletePaths || []}
          open={deletePaths !== null}
          onClose={() => setDeletePaths(null)}
          refresh={() => void queryClient.invalidateQueries({ queryKey: ["files"] })}
          onDeleted={() => setSelected(new Set())}
        />
      </div>
    </>
  );
}
