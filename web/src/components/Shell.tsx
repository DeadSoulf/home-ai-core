import type { ReactNode } from "react";
import type { RealtimeStatus } from "../api/client";
import type { Actor } from "../api/types";
import { LanguageSwitch, useI18n } from "../i18n";

const nav = [
  ["/", "dashboard"],
  ["/system", "system"],
  ["/modules", "modules"],
  ["/jobs", "jobs"],
  ["/audit", "audit"],
] as const;

export function Shell(props: {
  actor: Actor;
  path: string;
  realtime: RealtimeStatus;
  availableUpdate?: string;
  onNavigate: (path: string) => void;
  onLogout: () => void;
  children: ReactNode;
}) {
  const {t, status} = useI18n();
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">H</div>
          <div>
            <strong>Home-AI-Core</strong>
            <span>{t("controlPlane")}</span>
          </div>
        </div>

        <nav aria-label="Main navigation">
          {nav.map(([href, label]) => (
            <button
              key={href}
              className={props.path === href ? "nav-item active" : "nav-item"}
              onClick={() => props.onNavigate(href)}
            >
              {t(label)}
            </button>
          ))}
        </nav>

        <div className="sidebar-footer">
          <LanguageSwitch />
          <div className="connection-state">
            <span className={`status-dot ${props.realtime}`} />
            {t("realtime")}: {status(props.realtime)}
          </div>
          <div className="user-summary">
            <strong>{props.actor.display_name || props.actor.username || t("user")}</strong>
            <span>{props.actor.roles.join(", ") || props.actor.type}</span>
          </div>
          <button className="button secondary full" onClick={props.onLogout}>
            {t("signOut")}
          </button>
        </div>
      </aside>

      <main className="main-content">
        {props.availableUpdate && (
          <button
            type="button"
            className="update-banner"
            onClick={() => props.onNavigate("/system")}
          >
            <span>{t("newUpdateAvailable")}</span>
            <strong>{props.availableUpdate}</strong>
            <span className="update-banner-action">{t("openUpdate")}</span>
          </button>
        )}
        {props.children}
      </main>
    </div>
  );
}
