import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiURL, request, tasksSchema } from "../../lib/api";

// Durable snapshots replace the cache; disconnected streams fall back to polling.
export function useTaskEvents() {
  const client = useQueryClient();
  const [live, setLive] = useState(false);
  const query = useQuery({
    queryKey: ["tasks"],
    queryFn: ({ signal }) => request("/api/v1/uploads", tasksSchema, { signal }),
    refetchInterval: live ? false : 10000,
  });
  useEffect(() => {
    const stream = new EventSource(apiURL("/api/v1/events"));
    stream.addEventListener("tasks", (event) => {
      try {
        const data = tasksSchema.parse(JSON.parse((event as MessageEvent<string>).data));
        client.setQueryData(["tasks"], data);
        setLive(true);
      } catch {
        setLive(false);
      }
    });
    stream.onerror = () => setLive(false);
    return () => stream.close();
  }, [client]);
  return { live, query };
}
