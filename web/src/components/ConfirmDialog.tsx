import { useEffect, useId, useRef, type ReactNode } from "react";
import { useUI } from "../ui/useUI";
export function ConfirmDialog({title, children, confirmLabel, onConfirm, onCancel, confirmStyle = "danger"}: {
  title: string; children: ReactNode; confirmLabel: string; onConfirm: () => void; onCancel: () => void;
  confirmStyle?: "primary" | "danger";
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const label = useId(), description = useId();
  const u = useUI();
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    dialog.current?.showModal();
    return () => { dialog.current?.close(); if (previous?.isConnected) previous.focus(); };
  }, []);
  return <dialog ref={dialog} className="confirm-dialog" aria-labelledby={label} aria-describedby={description}
    onCancel={(event) => { event.preventDefault(); onCancel(); }}>
    <h2 id={label}>{title}</h2><div id={description}>{children}</div>
    <div className="button-row"><button autoFocus type="button" className="button secondary" onClick={onCancel}>{u("dismiss")}</button>
      <button type="button" className={"button " + confirmStyle} onClick={onConfirm}>{confirmLabel}</button></div>
  </dialog>;
}
