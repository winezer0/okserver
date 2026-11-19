import { UploadSelection } from "./upload-selection";
import { UploadTasks } from "./upload-tasks";

export function UploadPanel({ directory }: { directory: string }) {
  return (
    <div className="space-y-5">
      <UploadSelection directory={directory} />
      <UploadTasks />
    </div>
  );
}
