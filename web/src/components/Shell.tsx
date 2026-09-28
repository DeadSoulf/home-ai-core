import type { ReactNode } from "react";
import type { RealtimeStatus } from "../api/client";
import type { Actor } from "../api/types";

const nav = [
  ["/", "Dashboard"],
  ["/system", "System"],
  ["/modules", "Modules"],
  ["/jobs", "Jobs"],
  ["/audit", "Audit"],
] as const;

export function Shell(props: {
  actor: Actor;
  path: string;
  realtime: RealtimeStatus;
  onNavigate: (path: string) => void;
  onLogout: () => void;
  children: ReactNode;
}) {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">H</div>
          <div>
            <strong>Home-AI-Core</strong>
            <span>Control Plane</span>
          </div>
        </div>

        <nav aria-label="Main navigation">
          {nav.map(([href, label]) => (
            <button
              key={href}
              className={props.path === href ? "nav-item active" : "nav-item"}
              onClick={() => props.onNavigate(href)}
            >
              {label}
            </button>
          ))}
        </nav>

        <div className="sidebar-footer">
          <div className="connection-state">
            <span className={`status-dot ${props.realtime}`} />
            Realtime: {props.realtime}
          </div>
          <div className="user-summary">
            <strong>{props.actor.display_name || props.actor.username || "User"}</strong>
            <span>{props.actor.roles.join(", ") || props.actor.type}</span>
          </div>
          <button className="button secondary full" onClick={props.onLogout}>
            Sign out
          </button>
        </div>
      </aside>

      <main className="main-content">{props.children}</main>
    </div>
  );
}
