import type { ReactNode } from "react";
import { useUI } from "../ui/useUI";
import { useI18n } from "../i18n";

export function Panel(props: {
  title: string;
  children: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <section className={`panel ${props.className || ""}`.trim()}>
      <header className="panel-header">
        <h2>{props.title}</h2>
        {props.action}
      </header>
      {props.children}
    </section>
  );
}

export function EmptyState({children}: {children: ReactNode}) {
  return <div className="empty-state">{children}</div>;
}

export function ErrorState({message, onRetry}: {message?: string; onRetry?: () => void}) {
  const u = useUI();
  const {t} = useI18n();
  return <div className="error-state" role="alert">{message || t("loadError")}{onRetry && <button type="button" className="button secondary" onClick={onRetry}>{u("retry")}</button>}</div>;
}

export function LoadingState() {
  const {t} = useI18n();
  return <div className="loading-state" role="status" aria-live="polite">{t("loading")}</div>;
}
