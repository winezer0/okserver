import { useEffect, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { directorySchema, request } from "../../lib/api";

export function useDirectory() {
  const navigate = useNavigate();
  const directory = useParams()["*"] || "";
  const [parameters, setParameters] = useSearchParams();
  const page = Math.min(1000, Math.max(1, Number(parameters.get("page")) || 1));
  const limit = [50, 100, 200, 500].includes(Number(parameters.get("limit")))
    ? Number(parameters.get("limit"))
    : 100;
  const sort = ["name", "size", "modified", "type"].includes(parameters.get("sort") || "")
    ? parameters.get("sort")!
    : "name";
  const direction = parameters.get("direction") === "desc" ? "desc" : "asc";
  const search = parameters.get("search") || "";
  const [debouncedSearch, setDebouncedSearch] = useState(search);
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearch(search), 250);
    return () => clearTimeout(timer);
  }, [search]);
  const update = (name: string, value: string) =>
    setParameters(
      (current) => {
        const next = new URLSearchParams(current);
        next.set(name, value);
        if (name !== "page") next.delete("page");
        return next;
      },
      { replace: true },
    );
  const query = useQuery({
    queryKey: ["files", directory, page, limit, sort, direction, debouncedSearch],
    queryFn: ({ signal }) =>
      request(
        `/api/v1/files?${new URLSearchParams({ path: directory, page: String(page), limit: String(limit), sort, direction, search: debouncedSearch })}`,
        directorySchema,
        { signal },
      ),
  });
  const directoryLink = (path: string) =>
    `/files${path ? "/" + path.split("/").map(encodeURIComponent).join("/") : ""}${authSearch(parameters)}`;
  const parts = directory.split("/").filter(Boolean);
  return {
    navigate,
    directory,
    parameters,
    page,
    limit,
    sort,
    direction,
    search,
    update,
    query,
    directoryLink,
    parts,
  };
}
export type DirectoryModel = ReturnType<typeof useDirectory>;
function authSearch(parameters: URLSearchParams) {
  const auth = new URLSearchParams(parameters);
  for (const key of ["directory", "page", "limit", "search", "sort", "direction"]) auth.delete(key);
  return auth.size ? `?${auth}` : "";
}
