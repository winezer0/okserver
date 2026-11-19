import { useRef, useState } from "react";
import { UploadCloud } from "lucide-react";
import { Button } from "../../components/ui/button";
import { usePreferences } from "../../lib/preferences";
import { useUploadQueue } from "./use-upload-queue";

export function UploadSelection({ directory }: { directory: string }) {
  const { t } = usePreferences();
  const queue = useUploadQueue();
  const input = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  return (
    <div className="space-y-5">
      <div>
        <p className="mb-1 text-xs font-semibold tracking-widest text-primary">{t("transfer")}</p>
        <h2 id="upload-heading" className="text-lg font-semibold">
          {t("upload")}
        </h2>
        <p className="mt-1 text-xs text-muted-foreground">{t("uploadHint")}</p>
      </div>
      <input
        id="files"
        ref={input}
        type="file"
        multiple
        className="hidden"
        onChange={(event) => {
          if (event.target.files) queue.add(event.target.files, directory);
          event.target.value = "";
        }}
      />
      <div
        id="drop-zone"

        className={`rounded-xl border-2 border-dashed p-6 text-center transition-colors ${dragging ? "border-primary bg-primary/10" : "border-border bg-muted/30"}`}
        onDragOver={(event) => {
          event.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(event) => {
          event.preventDefault();
          event.stopPropagation();
          setDragging(false);
          queue.add(event.dataTransfer.files, directory);
        }}
      >
        <UploadCloud className="mx-auto mb-3 size-9 text-primary" aria-hidden="true" />
        <p className="font-medium">{t("drop")}</p>
        <p className="my-3 text-xs leading-relaxed text-muted-foreground">{t("multiple")}</p>
        <Button id="choose-files" variant="outline" onClick={() => input.current?.click()}>
          {t("choose")}
        </Button>
      </div>
      <p className="truncate rounded-lg bg-muted p-3 text-sm" title={`/${directory}`}>
        {t("destination")} <strong>/{directory}</strong>
      </p>
    </div>
  );
}
