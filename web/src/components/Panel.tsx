import type { ReactNode } from "react";

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
  return <div className="error-state">{message || "Unable to load data."}</div>;
}

export function LoadingState() {
  return <div className="loading-state">Loading…</div>;
}
