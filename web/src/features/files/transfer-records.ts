export interface DownloadHistory {
  id: string;
  path: string;
  name: string;
  size: number;
  completed: boolean;
  error?: string;
}
const key = "okserver:download-history:v1";
function load(): Map<string, DownloadHistory> {
  try {
    const saved: unknown = JSON.parse(sessionStorage.getItem(key) || "[]");
    if (!Array.isArray(saved)) return new Map();
    return new Map(
      saved
        .filter(
          (r): r is DownloadHistory =>
            !!r &&
            typeof r.id === "string" &&
            typeof r.path === "string" &&
            typeof r.name === "string" &&
            Number.isSafeInteger(r.size) &&
            r.size >= 0 &&
            typeof r.completed === "boolean",
        )
        .slice(-200)
        .map((r) => [r.id, r]),
    );
  } catch {
    return new Map();
  }
}
const records = load();
function save() {
  try {
    sessionStorage.setItem(key, JSON.stringify([...records.values()].slice(-200)));
  } catch {
    /* History cannot prevent transfer or cleanup. */
  }
}
export const downloadHistory = () => [...records.values()];
export const rememberDownload = (record: DownloadHistory) => {
  records.set(record.id, record);
  save();
};
export const removeDownloadHistory = (id: string) => {
  records.delete(id);
  save();
};
