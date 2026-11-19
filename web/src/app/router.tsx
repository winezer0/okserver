import {
  Link,
  Navigate,
  Outlet,
  createBrowserRouter,
  useLocation,
  useRouteError,
} from "react-router";
import { lazy, Suspense } from "react";
import { FolderUp, Languages, Moon, Sun } from "lucide-react";
import { usePreferences } from "../lib/preferences";
import { Button } from "../components/ui/button";
const DirectoryPage = lazy(() =>
  import("../features/files/directory-page").then((module) => ({ default: module.DirectoryPage })),
);
const TransferPage = lazy(() =>
  import("../features/files/transfer-page").then((module) => ({ default: module.TransferPage })),
);

export function Layout() {
  const { t, theme, toggleTheme, locale, toggleLocale } = usePreferences();
  const location = useLocation();
  return (
    <div className="mx-auto max-w-[1500px] px-4 md:px-8">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:z-50 rounded-lg bg-card p-3 text-primary"
      >
        {t("skip")}
      </a>
      <header className="flex min-h-20 flex-wrap items-center justify-between gap-3 border-b border-border py-3">
        <Link to={`/workspace/files${location.search}`} className="flex items-center gap-3">
          <span className="rounded-xl bg-primary p-2.5 text-primary-foreground">
            <FolderUp className="size-6" aria-hidden="true" />
          </span>
          <span>
            <strong className="text-lg font-semibold tracking-tight">okserver</strong>
            <small className="block text-[10px] tracking-[0.22em] text-muted-foreground">
              FILE WORKSPACE
            </small>
          </span>
        </Link>
        <nav className="flex flex-wrap items-center gap-2" aria-label={t("navigation")}>
          <Link
            className="nav-link"
            aria-current={location.pathname.startsWith("/workspace/files") ? "page" : undefined}
            to={`/workspace/files${location.search}`}
          >
            {t("files")}
          </Link>
          <Link
            className="nav-link"
            aria-current={location.pathname.endsWith("/tasks") ? "page" : undefined}
            to={`/workspace/tasks${location.search}`}
          >
            {locale === "zh" ? "传输中心" : "Transfer center"}
          </Link>
          <Button
            variant="ghost"
            id="language-toggle"
            aria-label={t("language")}
            onClick={toggleLocale}
          >
            <Languages aria-hidden="true" />
            {locale === "zh" ? "English" : "中文"}
          </Button>
          <Button
            variant="outline"
            id="theme-toggle"
            aria-pressed={theme === "dark"}
            aria-label={t("theme")}
            onClick={toggleTheme}
          >
            {theme === "light" ? <Moon aria-hidden="true" /> : <Sun aria-hidden="true" />}
          </Button>
        </nav>
      </header>
      <main id="main-content" className="py-7 md:py-9">
        <div className="mb-7">
          <p className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-primary">
            {t("tagline")}
          </p>
          <h1 className="text-2xl font-semibold tracking-tight md:text-3xl">
            {location.pathname.endsWith("/tasks")
              ? locale === "zh"
                ? "传输中心"
                : "Transfer center"
              : t("workspace")}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {location.pathname.endsWith("/tasks")
              ? locale === "zh"
                ? "查看传输进度，恢复中断任务或清理暂存。"
                : "Track transfers, resume interrupted tasks, and clean staging."
              : t("description")}
          </p>
        </div>
        <Suspense fallback={<p role="status">{t("loading")}</p>}>
          <Outlet />
        </Suspense>
      </main>
      <footer className="border-t border-border py-6 text-xs text-muted-foreground">
        okserver · {t("storage")}
      </footer>
    </div>
  );
}
function InitialRoute() {
  const { search } = useLocation();
  const parameters = new URLSearchParams(search);
  const directory = parameters.get("directory") || "";
  parameters.delete("directory");
  return (
    <Navigate
      replace
      to={`/workspace/files${directory ? "/" + directory.split("/").map(encodeURIComponent).join("/") : ""}${parameters.size ? "?" + parameters : ""}`}
    />
  );
}
export function ErrorPage() {
  const { t } = usePreferences();
  const error = useRouteError();
  return (
    <main className="mx-auto max-w-lg px-5 py-24 text-center">
      <h1 className="text-2xl font-semibold">{t(error ? "error" : "notFound")}</h1>
      <Link className="nav-link mt-6 inline-block text-primary" to="/workspace/files">
        {t("back")}
      </Link>
    </main>
  );
}
export const routes = [
  { path: "/", element: <InitialRoute /> },
  {
    path: "/workspace",
    element: <Layout />,
    errorElement: <ErrorPage />,
    children: [
      { index: true, element: <Navigate to="files" replace /> },
      { path: "files/*", element: <DirectoryPage /> },
      {
        path: "tasks",
        element: (
          <div>
            <TransferPage />
          </div>
        ),
      },
      { path: "*", element: <ErrorPage /> },
    ],
  },
  { path: "*", element: <ErrorPage /> },
];
export const createRouter = () => createBrowserRouter(routes);
