import { Link } from "react-router";
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
  type Row,
} from "@tanstack/react-table";
import { Download, File, Folder, Trash2 } from "lucide-react";
import { apiURL, type FileEntry } from "../../lib/api";
import { usePreferences } from "../../lib/preferences";
import { formatBytes } from "../../lib/utils";
import { downloadVerified } from "./download";
import { useState, useRef } from "react";
import { Button } from "../../components/ui/button";

interface Props {
  entries: FileEntry[];
  total: number;
  directoryLink: (path: string) => string;
  allowDelete: boolean;
  selectMode: boolean;
  selected: Set<string>;
  onToggle: (path: string) => void;
  onDeletePath: (path: string) => void;
}
export function FileTable({
  entries,
  total,
  directoryLink,
  allowDelete,
  selectMode,
  selected,
  onToggle,
  onDeletePath,
}: Props) {
  const [downloadStatus, setDownloadStatus] = useState("");
  const [downloading, setDownloading] = useState(false);
  const downloadController = useRef<AbortController | null>(null);
  const { t, locale } = usePreferences();
  const columns = [
    ...(selectMode
      ? [
          {
            id: "select",
            header: "",
            cell: ({ row }: { row: Row<FileEntry> }) => (
              <input
                type="checkbox"
                aria-label={`${t("selectMode")} ${row.original.name}`}
                checked={selected.has(row.original.path)}
                onChange={() => onToggle(row.original.path)}
                className="size-4 accent-primary"
              />
            ),
          },
        ]
      : []),
    {
      accessorKey: "name",
      header: t("name"),
      cell: ({ row }) => (
        <div className="flex min-w-0 items-center gap-3">
          {row.original.is_dir ? (
            <Folder className="size-5 shrink-0 text-primary" aria-hidden="true" />
          ) : (
            <File className="size-5 shrink-0 text-muted-foreground" aria-hidden="true" />
          )}
          <Link
            className="entry-name truncate font-medium hover:text-primary hover:underline"
            to={
              row.original.is_dir
                ? directoryLink(row.original.path)
                : apiURL(
                    `/download/${row.original.path.split("/").map(encodeURIComponent).join("/")}`,
                  )
            }
            reloadDocument={!row.original.is_dir}
            onClick={(event) => {
              if (!row.original.is_dir) {
                event.preventDefault();
                event.currentTarget
                  .closest("tr")
                  ?.querySelector<HTMLAnchorElement>("a.row-action")
                  ?.click();
              }
            }}
            download={!row.original.is_dir || undefined}
            title={row.original.name}
          >
            {row.original.name}
          </Link>
        </div>
      ),
    },
    {
      accessorKey: "size",
      header: t("size"),
      cell: ({ row }) => (row.original.is_dir ? "—" : formatBytes(row.original.size)),
    },
    {
      accessorKey: "modified",
      header: t("modified"),
      cell: ({ row }) =>
        new Date(row.original.modified).toLocaleString(locale === "zh" ? "zh-CN" : "en-US"),
    },
    {
      id: "action",
      header: t("action"),
      cell: ({ row }) => (
        <div className="flex items-center gap-3">
          {row.original.is_dir ? (
            <Link className="row-action" to={directoryLink(row.original.path)}>
              {t("open")}
            </Link>
          ) : (
            <a
              className="row-action"
              aria-label={`${t("download")} ${row.original.name}`}
              download
              href={apiURL(
                `/download/${row.original.path.split("/").map(encodeURIComponent).join("/")}`,
              )}
              onClick={(event) => {
                if (
                  !("showSaveFilePicker" in window) ||
                  !navigator.storage?.getDirectory ||
                  !navigator.locks
                )
                  return;
                event.preventDefault();
                if (downloadController.current) return;
                const active = new AbortController();
                downloadController.current = active;
                setDownloading(true);
                setDownloadStatus(t("loading"));
                void downloadVerified(
                  row.original.path,
                  row.original.name,
                  (text) => setDownloadStatus(`${row.original.name} · ${text}`),
                  active.signal,
                )
                  .catch((error: unknown) =>
                    setDownloadStatus(
                      active.signal.aborted
                        ? t("paused")
                        : `${t("requestFailed")}: ${String(error)}`,
                    ),
                  )
                  .finally(() => {
                    downloadController.current = null;
                    setDownloading(false);
                  });
              }}
            >
              <Download className="size-4" aria-hidden="true" />
              <span className="sr-only">{t("download")}</span>
            </a>
          )}
          {allowDelete && (
            <Button
              type="button"
              variant="ghost"
              aria-label={`${t("deleteItem")} ${row.original.name}`}
              title={t("deleteItem")}
              onClick={() => onDeletePath(row.original.path)}
            >
              <Trash2 className="size-4 text-destructive" aria-hidden="true" />
            </Button>
          )}
        </div>
      ),
    },
  ] satisfies ColumnDef<FileEntry>[];
  const table = useReactTable({
    data: entries,
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    manualSorting: true,
    rowCount: total,
  });
  return (
    <div className="overflow-x-auto">
      {downloadStatus && (
        <p role="status" className="mb-3 rounded-lg bg-muted p-3 text-sm">
          {t("download")} · {downloadStatus}
        </p>
      )}
      {downloading && (
        <Button variant="outline" onClick={() => downloadController.current?.abort()}>
          {t("cancel")} {t("download")}
        </Button>
      )}
      <table className="w-full text-left text-sm">
        <thead>
          {table.getHeaderGroups().map((group) => (
            <tr key={group.id}>
              {group.headers.map((header) => (
                <th
                  key={header.id}
                  scope="col"
                  className="border-b border-border py-3 pr-4 font-medium text-muted-foreground"
                >
                  {flexRender(header.column.columnDef.header, header.getContext())}
                </th>
              ))}
            </tr>
          ))}
        </thead>
        <tbody id="listing">
          {table.getRowModel().rows.map((row) => (
            <tr key={row.original.path} className="hover:bg-muted/40">
              {row.getVisibleCells().map((cell) => (
                <td
                  key={cell.id}
                  className="max-w-64 border-b border-border/60 py-4 pr-4 last:pr-0"
                >
                  {flexRender(cell.column.columnDef.cell, cell.getContext())}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
