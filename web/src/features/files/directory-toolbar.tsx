import { Link } from "react-router";
import { ArrowDownUp, ArrowLeft, ChevronLeft, ChevronRight, RefreshCw, Search } from "lucide-react";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { usePreferences } from "../../lib/preferences";
import type { DirectoryModel } from "./use-directory";

export function DirectoryToolbar({
  model,
  onUpload,
  allowCreateDir = false,
  allowDeleteFile = false,
  onCreateDirectory,
  selectedCount = 0,
  pageCount = 0,
  selectMode = false,
  onToggleSelectMode,
  onSelectPage,
  onDeleteSelected,
  onDownloadSelected,
}: {
  model: DirectoryModel;
  onUpload?: () => void;
  allowCreateDir?: boolean;
  allowDeleteFile?: boolean;
  onCreateDirectory?: () => void;
  selectedCount?: number;
  pageCount?: number;
  selectMode?: boolean;
  onToggleSelectMode?: () => void;
  onSelectPage?: () => void;
  onDeleteSelected?: () => void;
  onDownloadSelected?: () => void;
}) {
  const { t } = usePreferences();
  const { directory, navigate, directoryLink, parts, query, search, sort, direction, update } =
    model;
  return (
    <>
      {" "}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="files-heading" className="text-lg font-semibold">
          {t("directory")}
        </h2>
        <div className="flex gap-2">
          {onUpload && (
            <Button id="open-upload" onClick={onUpload}>
              {t("upload")}
            </Button>
          )}
          {allowCreateDir && onCreateDirectory && (
            <Button id="create-directory" variant="outline" onClick={onCreateDirectory}>
              {t("createDirectory")}
            </Button>
          )}
          {onToggleSelectMode && (
            <Button
              id="toggle-select-mode"
              variant="outline"
              aria-pressed={selectMode}
              onClick={onToggleSelectMode}
            >
              {selectMode ? t("exitSelectMode") : t("selectMode")}
            </Button>
          )}
          {selectMode && pageCount > 0 && onSelectPage && (
            <Button id="select-page" variant="outline" onClick={onSelectPage}>
              {t("selectPage")}
            </Button>
          )}
          {selectedCount > 0 && onDownloadSelected && (
            <Button id="download-selected" variant="outline" onClick={onDownloadSelected}>
              {t("downloadSelected")} ({selectedCount})
            </Button>
          )}
          {allowDeleteFile && selectedCount > 0 && onDeleteSelected && (
            <Button id="delete-selected" variant="destructive" onClick={onDeleteSelected}>
              {t("deleteSelected")} ({selectedCount})
            </Button>
          )}
          <Button
            id="up-directory"
            variant="outline"
            disabled={!directory}
            onClick={() => navigate(directoryLink(parts.slice(0, -1).join("/")))}
          >
            <ArrowLeft aria-hidden="true" />
            {t("parent")}
          </Button>
          <Button
            id="refresh-files"
            variant="ghost"
            aria-label={t("refresh")}
            onClick={() => void query.refetch()}
          >
            <RefreshCw className={query.isFetching ? "animate-spin" : ""} aria-hidden="true" />
          </Button>
        </div>
      </div>
      <nav
        id="breadcrumbs"
        aria-label={t("breadcrumb")}
        className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground"
      >
        <Link to={directoryLink("")}>{t("root")}</Link>
        {parts.map((part, index) => (
          <span key={index} className="flex items-center gap-2">
            <ChevronRight className="size-3" aria-hidden="true" />
            <Link
              to={directoryLink(parts.slice(0, index + 1).join("/"))}
              aria-current={index === parts.length - 1 ? "location" : undefined}
            >
              {part}
            </Link>
          </span>
        ))}
      </nav>
      <div className="flex flex-wrap gap-3">
        <label className="relative flex-1">
          <Search
            className="absolute left-3 top-3.5 size-4 text-muted-foreground"
            aria-hidden="true"
          />
          <span className="sr-only">{t("search")}</span>
          <Input
            id="search-files"
            type="search"
            className="w-full pl-10"
            placeholder={t("search")}
            value={search}
            onChange={(event) => update("search", event.target.value)}
          />
        </label>
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          {t("sort")}
          <select
            id="sort-files"
            value={sort}
            onChange={(event) => update("sort", event.target.value)}
          >
            {(["name", "size", "modified", "type"] as const).map((key) => (
              <option key={key} value={key}>
                {t(key)}
              </option>
            ))}
          </select>
        </label>
        <Button
          id="sort-direction"
          variant="outline"
          aria-label={t("direction")}
          onClick={() => update("direction", direction === "asc" ? "desc" : "asc")}
        >
          <ArrowDownUp aria-hidden="true" />
        </Button>
      </div>
    </>
  );
}
export function DirectoryPagination({ model }: { model: DirectoryModel }) {
  const { t } = usePreferences();
  const { query, page, limit, update } = model;
  return (
    <nav className="flex flex-wrap items-center justify-between gap-3" aria-label={t("page")}>
      <span id="page-summary" className="text-xs text-muted-foreground" aria-live="polite">
        {t("page")} {page} · {query.data?.total || 0} {t("entries")}
      </span>
      <div className="flex items-center gap-2">
        <label className="text-xs text-muted-foreground">
          <span className="sr-only">{t("perPage")}</span>
          <select
            id="page-size"
            value={limit}
            onChange={(event) => update("limit", event.target.value)}
          >
            {[50, 100, 200, 500].map((value) => (
              <option key={value}>{value}</option>
            ))}
          </select>
        </label>
        <Button
          id="previous-page"
          hidden={(query.data?.total || 0) <= limit}
          variant="outline"
          aria-label={t("previous")}
          disabled={page <= 1}
          onClick={() => update("page", String(page - 1))}
        >
          <ChevronLeft aria-hidden="true" />
        </Button>
        <Button
          id="next-page"
          hidden={(query.data?.total || 0) <= limit}
          variant="outline"
          aria-label={t("next")}
          disabled={!query.data || page * limit >= query.data.total || page >= 1000}
          onClick={() => update("page", String(page + 1))}
        >
          <ChevronRight aria-hidden="true" />
        </Button>
      </div>
    </nav>
  );
}
