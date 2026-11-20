import { z } from "zod";
import { uploadSchema } from "./schemas";

export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
    public retryAfterMS = 0,
    public code?: string,
  ) {
    super(message);
  }
}
export function apiURL(value: string) {
  const url = new URL(value, location.origin);
  // Preserve only deployment authentication parameters, never UI route parameters.
  for (const [name, value] of new URLSearchParams(location.search)) {
    if (
      ![
        "directory",
        "page",
        "search",
        "sort",
        "direction",
        "limit",
        "sha256",
        "size",
        "prefix",
        "overwrite",
        "takeover",
      ].includes(name) &&
      !url.searchParams.has(name)
    )
      url.searchParams.append(name, value);
  }
  return url.pathname + url.search;
}
export function retryAfterMS(response: Response): number {
  const value = response.headers.get("Retry-After");
  if (!value) return 0;
  const delay = /^\d+$/.test(value) ? Number(value) * 1000 : Date.parse(value) - Date.now();
  return Number.isFinite(delay) ? Math.max(0, delay) : 0;
}
const apiErrorSchema = z.object({ error: z.string(), code: z.string().optional() });
export async function apiErrorFromResponse(response: Response): Promise<APIError> {
  const body: unknown = await response.clone().json().catch(() => null);
  const parsed = apiErrorSchema.safeParse(body);
  return new APIError(
    response.status,
    parsed.success ? parsed.data.error : `HTTP ${response.status}`,
    retryAfterMS(response),
    parsed.success ? parsed.data.code : undefined,
  );
}
export async function request<T>(
  url: string,
  schema: z.ZodType<T>,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(apiURL(url), options);
  if (!response.ok) throw await apiErrorFromResponse(response);
  const body: unknown = response.status === 204 ? {} : await response.json().catch(() => null);
  return schema.parse(body);
}
export const requestUpload = (url: string, options?: RequestInit) =>
  request(url, uploadSchema, options);
export async function cancelUpload(id: string): Promise<void> {
  for (let attempt = 0; ; attempt++) {
    try {
      const tasks = await request(
        "/api/v1/uploads",
        z.object({ uploads: z.array(uploadSchema).nullable() }),
      );
      const task = tasks.uploads?.find((value) => value.id === id);
      const token = task
        ? localStorage.getItem(`okserver:upload-token:${location.origin}:${task.path}`)
        : null;
      await request(`/api/v1/uploads/${encodeURIComponent(id)}`, z.object({}), {
        method: "DELETE",
        headers: token ? { "Upload-Token": token } : {},
        signal: AbortSignal.timeout(15000),
      });
      return;
    } catch (error) {
      if (error instanceof APIError && error.status === 404) return;
      if (
        attempt === 2 ||
        !retryQuery(attempt, error instanceof Error ? error : new Error(String(error)))
      )
        throw error;
      await new Promise((resolve) => setTimeout(resolve, 300 * (attempt + 1)));
    }
  }
}
export function isRetryable(error: unknown, reconcileConflict = false) {
  if (error instanceof z.ZodError) return false;
  if (typeof error === "object" && error !== null && "name" in error && error.name === "AbortError")
    return false;
  return (
    !(error instanceof APIError) ||
    (error.status >= 500 && error.status <= 599) ||
    [408, 423, 429, ...(reconcileConflict ? [409] : [])].includes(error.status)
  );
}
export function retryQuery(failures: number, error: Error) {
  return failures < 2 && isRetryable(error);
}
