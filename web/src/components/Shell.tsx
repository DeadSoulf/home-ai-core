import { useEffect, useRef, useState, type ReactNode } from "react";
import type { RealtimeStatus } from "../api/client";
import type { Actor } from "../api/types";
import { LanguageSwitch, useI18n } from "../i18n";
import { hasPermission, visibleNavigation } from "../navigation";

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
  const [menuOpen, setMenuOpen] = useState(false);
  const wasOpen = useRef(false);
  const menuButton = useRef<HTMLButtonElement>(null);
  const sidebar = useRef<HTMLElement>(null);
  const groups = visibleNavigation(props.actor);
  const current = groups.flatMap((group) => group.items).find((item) => item.path === props.path.split("#")[0]);
  const closeMenu = () => {
    setMenuOpen(false);
  };

  useEffect(() => {
    const media = window.matchMedia("(max-width: 760px)");
    const resized = () => { if (!media.matches) setMenuOpen(false); };
    media.addEventListener("change", resized);
    return () => media.removeEventListener("change", resized);
  }, []);

  useEffect(() => {
    if (!menuOpen) {
      if (wasOpen.current) menuButton.current?.focus();
      wasOpen.current = false;
      return;
    }
    wasOpen.current = true;
    sidebar.current?.querySelector<HTMLButtonElement>(".nav-item.active")?.focus();
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setMenuOpen(false);
      }
      if (event.key === "Tab" && window.matchMedia("(max-width: 760px)").matches) {
        const buttons = sidebar.current?.querySelectorAll<HTMLButtonElement>("button");
        if (!buttons?.length) return;
        const first = buttons[0];
        const last = buttons[buttons.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault(); last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault(); first.focus();
        }
      }
    };
    window.addEventListener("keydown", keydown);
    const previousOverflow = document.body.style.overflow;
    if (window.matchMedia("(max-width: 760px)").matches) document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", keydown);
      document.body.style.overflow = previousOverflow;
    };
  }, [menuOpen]);

  return (
    <div className="app-shell">
      <header className="mobile-header" inert={menuOpen}>
        <button ref={menuButton} type="button" className="button secondary" aria-expanded={menuOpen}
          aria-controls="main-sidebar" onClick={() => setMenuOpen(true)}>{t("menu")}</button>
        <strong>{t(current?.label || "home")}</strong>
        <img className="brand-logo" src="/brand/logo-mark.webp" alt="Home-AI-Core" />
      </header>
      {menuOpen && <button type="button" className="menu-backdrop" aria-label={t("closeMenu")} onClick={closeMenu} />}
      <aside ref={sidebar} id="main-sidebar" className={`sidebar${menuOpen ? " is-open" : ""}`}
        role={menuOpen ? "dialog" : undefined} aria-modal={menuOpen || undefined} aria-label={t("mainNavigation")}>
        <div className="brand">
          <img className="brand-logo" src="/brand/logo-mark.webp" alt="" />
          <strong>Home-AI-Core</strong>
          <button type="button" className="menu-close button secondary" onClick={closeMenu} aria-label={t("closeMenu")}>×</button>
        </div>
        <nav aria-label={t("mainNavigation")}>
          {groups.map((group) => (
            <div className="nav-group" key={group.label}>
              {group.label !== "home" && <span className="nav-group-label">{t(group.label)}</span>}
              {group.items.map(({path, label}) => (
                <button type="button" key={path} className={props.path.split("#")[0] === path ? "nav-item active" : "nav-item"}
                  aria-current={props.path.split("#")[0] === path ? "page" : undefined}
                  onClick={() => { props.onNavigate(path); if (menuOpen) closeMenu(); }}>
                  <span>{t(label)}</span>
                  {path === "/system" && props.availableUpdate && hasPermission(props.actor, "updates.read") &&
                    <span className="nav-update-badge" aria-label={t("newUpdateAvailable")}>↑</span>}
                </button>
              ))}
            </div>
          ))}
        </nav>
        <div className="sidebar-footer">
          <div className="account-row">
            <strong>{props.actor.display_name || props.actor.username || t("user")}</strong>
            {hasPermission(props.actor, "events.read") && <span className={`status-dot ${props.realtime}`} title={status(props.realtime)} aria-label={status(props.realtime)} />}
          </div>
          {hasPermission(props.actor, "events.read") && props.realtime !== "connected" &&
            <span className="connection-state">{t("connectionLost")}</span>}
          <div className="account-actions">
            <LanguageSwitch />
            <button type="button" className="button secondary" onClick={props.onLogout}>{t("signOut")}</button>
          </div>
        </div>
      </aside>
      <main className="main-content" inert={menuOpen}>
        {props.availableUpdate && hasPermission(props.actor, "updates.read") && hasPermission(props.actor, "system.read") &&
          <button type="button" className="update-banner" onClick={() => props.onNavigate("/system#updates")}>
            <span>{t("newUpdateAvailable")}</span><strong>{props.availableUpdate}</strong>
            <span className="update-banner-action">{t("openUpdate")}</span>
          </button>}
        {props.children}
      </main>
    </div>
  );
}
