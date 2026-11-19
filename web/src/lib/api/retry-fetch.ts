import { APIError, retryAfterMS } from "./client";
import { delay } from "../../features/uploads/transfer";

export async function retryFetch(url: string, options: RequestInit): Promise<Response> {
  for (let attempt = 0; ; attempt++) {
    options.signal?.throwIfAborted();
    try {
      const response = await fetch(url, options);
      if (![408, 423, 429].includes(response.status) && response.status < 500) return response;
      const error = new APIError(
        response.status,
        `HTTP ${response.status}`,
        retryAfterMS(response),
      );
      await response.body?.cancel();
      throw error;
    } catch (error) {
      if (
        attempt >= 5 ||
        options.signal?.aborted ||
        !(error instanceof TypeError || error instanceof APIError)
      )
        throw error;
      await delay(
        Math.max(
          Math.min(30000, 500 * 2 ** attempt) * (0.5 + Math.random()),
          error instanceof APIError ? error.retryAfterMS : 0,
        ),
        options.signal ?? undefined,
      );
    }
  }
}
