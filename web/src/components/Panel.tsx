import type { ReactNode } from "react";
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

export function ErrorState({message}: {message?: string}) {
  const {t} = useI18n();
  return <div className="error-state">{message || t("loadError")}</div>;
}

export function LoadingState() {
  const {t} = useI18n();
  return <div className="loading-state">{t("loading")}</div>;
}
