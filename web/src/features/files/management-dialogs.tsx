import { FormEvent, useEffect, useRef, useState } from "react";
import { z } from "zod";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { request } from "../../lib/api";
import { usePreferences } from "../../lib/preferences";

function Modal({
  title,
  open,
  onClose,
  children,
}: {
  title: string;
  open: boolean;
  onClose: () => void;
  children: React.ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (open && !ref.current?.open) ref.current?.showModal();
    if (!open && ref.current?.open) ref.current?.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onCancel={onClose}
      aria-label={title}
      className="m-auto w-[calc(100%_-_2rem)] max-w-md rounded-xl border border-border bg-background p-5 text-foreground shadow-xl backdrop:bg-black/40"
    >
      <h2 className="mb-4 text-lg font-semibold">{title}</h2>
      {children}
    </dialog>
  );
}

export function CreateDirectoryDialog({
  directory,
  open,
  onClose,
  refresh,
}: {
  directory: string;
  open: boolean;
  onClose: () => void;
  refresh: () => void;
}) {
  const { t } = usePreferences();
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await request("/api/v1/create", z.object({ path: z.string() }), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ parent: directory, name }),
      });
      refresh();
      setName("");
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal title={t("createDirectory")} open={open} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <label className="block text-sm">
          {t("directoryName")}
          <Input
            autoFocus
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            className="mt-2 w-full"
          />
        </label>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button disabled={busy}>{busy ? t("loading") : t("create")}</Button>
        </div>
      </form>
    </Modal>
  );
}

export function DeleteConfirmDialog({
  paths,
  open,
  onClose,
  refresh,
  onDeleted,
}: {
  paths: string[];
  open: boolean;
  onClose: () => void;
  refresh: () => void;
  onDeleted?: () => void;
}) {
  const { t } = usePreferences();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    setBusy(true);
    setError("");
    try {
      await request("/api/v1/delete", z.object({ deleted: z.array(z.string()) }), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ paths }),
      });
      refresh();
      onDeleted?.();
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal title={t("confirmDelete")} open={open} onClose={onClose}>
      <p className="mb-3 text-sm">{t("recursiveDeleteNotice")}</p>
      <p className="mb-4 max-h-36 overflow-auto rounded-md bg-muted p-3 text-sm">
        {paths.join(", ")}
      </p>
      {error && (
        <p role="alert" className="mb-3 text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={onClose}>
          {t("cancel")}
        </Button>
        <Button
          variant="destructive"
          disabled={busy || !paths.length}
          onClick={() => void submit()}
        >
          {busy ? t("loading") : t("delete")}
        </Button>
      </div>
    </Modal>
  );
}
