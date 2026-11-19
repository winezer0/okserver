import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "react-router/dom";
import { createRouter } from "./app";
import { Preferences } from "./lib/preferences";
import { retryQuery } from "./lib/api";
import { UploadQueueProvider } from "./features/uploads/use-upload-queue";
import "./styles.css";

const client = new QueryClient({
  defaultOptions: { queries: { retry: retryQuery, staleTime: 5000 }, mutations: { retry: false } },
});
createRoot(document.getElementById("root")!).render(
  <QueryClientProvider client={client}>
    <Preferences>
      <UploadQueueProvider>
        <RouterProvider router={createRouter()} />
      </UploadQueueProvider>
    </Preferences>
  </QueryClientProvider>,
);
