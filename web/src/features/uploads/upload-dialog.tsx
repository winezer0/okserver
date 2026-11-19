import { useEffect, useRef } from "react";
import { Button } from "../../components/ui/button";
import { usePreferences } from "../../lib/preferences";
import { UploadPanel } from "./upload-panel";

export function UploadDialog({
  directory,
  open,
  onClose,
}: {
  directory: string;
  open: boolean;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const { t, locale } = usePreferences();
  useEffect(() => {
    if (open && !ref.current?.open) ref.current?.showModal();
    if (!open && ref.current?.open) ref.current?.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onCancel={onClose}
      aria-label={t("upload")}
      className="m-auto max-h-[90dvh] w-[calc(100%_-_2rem)] max-w-xl overflow-y-auto rounded-xl border border-border bg-background p-4 text-foreground shadow-xl backdrop:bg-black/40"
    >
      <div className="mb-3 flex justify-end">
        <Button id="close-upload" variant="outline" onClick={onClose}>
          {locale === "zh" ? "关闭" : "Close"}
        </Button>
      </div>
      <UploadPanel directory={directory} />
    </dialog>
  );
}
