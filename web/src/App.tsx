import { useEffect, useRef, useState } from "react";
import { api, APIError, connectRealtime, setCSRFToken, type RealtimeStatus } from "./api/client";
import { ErrorNotice } from "./components/ErrorNotice";
import { LoadingState } from "./components/Panel";
import { useUI } from "./ui/useUI";
import { ModulePage } from "./pages/Module";
import { isModuleRoute } from "./navigation";
import type { Actor, ModuleNavigationState } from "./api/types";
import { Shell } from "./components/Shell";
import { useI18n } from "./i18n";
import { accessiblePath, fileSection, fileSectionPath, systemSection } from "./navigation";
import { FirstRunPage, LoginPage } from "./pages/Auth";
import { AccountPage } from "./pages/Account";
import { AuditPage } from "./pages/Audit";
import { Dashboard } from "./pages/Dashboard";
import { FilesPage } from "./pages/Files";
import { JobsPage } from "./pages/Jobs";
import { ModulesPage } from "./pages/Modules";
import { SystemPage } from "./pages/System";
import { UsersPage } from "./pages/Users";

type Phase = "loading" | "unavailable" | "setup" | "login" | "app";

function currentPath(): string {
  const path = window.location.pathname.replace(/\/+$/, "") || "/";
  const known = ["/", "/files", "/files/storage", "/files/windows", "/system", "/modules", "/jobs", "/audit", "/users", "/account"];
  const moduleRoute = isModuleRoute(path);
  if (!known.includes(path) && !moduleRoute) return "/";
  return path + (path === "/system" || path === "/files" || path === "/jobs" ? window.location.hash : "");
}


export default function App() {
  const {t} = useI18n();
  const u = useUI();
  const [startupRevision, retryStartup] = useState(0);
  const [startupError, setStartupError] = useState<unknown>();
  const [logoutError, setLogoutError] = useState<unknown>();
  const [loggingOut, setLoggingOut] = useState(false);
  const logoutLock = useRef(false);
  const [phase, setPhase] = useState<Phase>("loading");
  const [actor, setActor] = useState<Actor>();
  const [path, setPath] = useState(currentPath);
  const [realtime, setRealtime] = useState<RealtimeStatus>("disconnected");
  const [revision, setRevision] = useState(0);
  const [availableUpdate, setAvailableUpdate] = useState<string>();
  const [moduleNavigation, setModuleNavigation] = useState<ModuleNavigationState[]>();
  const [navigationError, setNavigationError] = useState<unknown>();

  useEffect(() => {
    let stopped = false;
    const controller = new AbortController();
    setPhase("loading"); setStartupError(undefined);
    (async () => {
      try {
        const status = await api.setupStatus(controller.signal);
        if (stopped) return;
        if (!status.initialized) { setPhase("setup"); return; }
        const current = await api.me(controller.signal);
        if (!stopped) { setActor(current); setPhase("app"); }
      } catch (error) {
        if (stopped) return;
        if (error instanceof APIError && error.status === 401) { setActor(undefined); setPhase("login"); }
        else { setStartupError(error); setPhase("unavailable"); }
      }
    })();
    return () => { stopped = true; controller.abort(); };
  }, [startupRevision]);

  useEffect(() => {
    const pop = () => setPath(currentPath());
    window.addEventListener("popstate", pop);
    window.addEventListener("hashchange", pop);
    return () => { window.removeEventListener("popstate", pop); window.removeEventListener("hashchange", pop); };
  }, []);

  useEffect(() => {
    if (phase !== "app" || !actor?.permissions.includes("events.read")) {
      setRealtime("disconnected");
      return;
    }
    return connectRealtime(
      (event) => {
        if (!event.type.startsWith("core.")) {
          setRevision((value) => value + 1);
        }
        if (event.type === "module.runtime.changed") {
          window.dispatchEvent(new CustomEvent("home-ai-core:modules-changed"));
        }
      },
      setRealtime,
    );
  }, [phase, actor]);

  useEffect(() => {
    if (phase !== "app" || !actor?.permissions.includes("updates.read")) {
      setAvailableUpdate(undefined);
      return;
    }
    let stopped = false;
    let checking = false;
    const check = async (fresh = false) => {
      if (checking) return;
      checking = true;
      try {
        const result = await api.updateStatus(fresh);
        if (!stopped) {
          setAvailableUpdate(result.available ? result.available_version : undefined);
        }
      } catch {
        // Update notifications are best-effort and must not interrupt the UI.
      } finally {
        checking = false;
      }
    };
    const refreshWhenVisible = () => {
      if (document.visibilityState === "visible") void check(false);
    };
    const updateStatusEvent = (event: Event) => {
      const detail = (event as CustomEvent<string | undefined>).detail;
      setAvailableUpdate(detail || undefined);
    };

    void check(true);
    const timer = window.setInterval(() => { void check(true); }, 5 * 60 * 1000);
    window.addEventListener("focus", refreshWhenVisible);
    document.addEventListener("visibilitychange", refreshWhenVisible);
    window.addEventListener("home-ai-core:update-status", updateStatusEvent);
    return () => {
      stopped = true;
      window.clearInterval(timer);
      window.removeEventListener("focus", refreshWhenVisible);
      document.removeEventListener("visibilitychange", refreshWhenVisible);
      window.removeEventListener("home-ai-core:update-status", updateStatusEvent);
    };
  }, [phase, actor]);

  useEffect(() => {
    if (phase !== "app" || !actor) {
      setModuleNavigation(undefined); setNavigationError(undefined);
      return;
    }
    let stopped = false, checking = false;
    const refresh = async () => {
      if (checking) return;
      checking = true;
      try {
        const items = await api.moduleNavigation();
        if (!stopped) {setModuleNavigation(items); setNavigationError(undefined);}
      } catch (error) {
        if (!stopped) setNavigationError(error);
      } finally {checking = false;}
    };
    void refresh();
    const changed = () => { void refresh(); };
    window.addEventListener("home-ai-core:modules-changed", changed);
    const timer = window.setInterval(changed, 10000);
    return () => {
      stopped = true;
      window.removeEventListener("home-ai-core:modules-changed", changed);
      window.clearInterval(timer);
    };
  }, [phase, actor]);

  useEffect(() => {
    if (phase !== "app" || !actor || !moduleNavigation || isModuleRoute(path)) return;
    const allowedPath = accessiblePath(actor, path, moduleNavigation);
    if (allowedPath !== path) {
      window.history.replaceState({}, "", allowedPath);
      setPath(allowedPath);
    }
  }, [phase, actor, path, moduleNavigation]);


  const authenticated = (nextActor: Actor) => {
    setActor(nextActor);
    setPhase("app");
  };

  const logout = async () => {
    if (logoutLock.current) return;
    logoutLock.current = true; setLoggingOut(true); setLogoutError(undefined);
    try {
      try { await api.logout(); }
      catch (error) { if (!(error instanceof APIError && error.status === 401)) throw error; }
      setCSRFToken(); setActor(undefined); setPhase("login");
    } catch (error) { setLogoutError(error); }
    finally { logoutLock.current = false; setLoggingOut(false); }
  };

  const navigate = (nextPath: string) => {
    const allowed = actor ? accessiblePath(actor, nextPath, moduleNavigation) : "/";
    if (allowed !== path) {
      window.history.pushState({}, "", allowed);
      setPath(allowed);
    }
  };

  if (phase === "loading") {
    return <div className="boot-screen" role="status"><div className="brand-mark">H</div><span>{t("starting")}</span></div>;
  }
  if (phase === "unavailable") return <main className="boot-screen connection-error">
    <h1>{u("unavailable")}</h1><p>{u("unavailableHint")}</p><ErrorNotice error={startupError} />
    <button className="button primary" type="button" onClick={() => retryStartup((value) => value + 1)}>{u("retry")}</button>
  </main>;
  if (phase === "setup") {
    return <FirstRunPage onAuthenticated={authenticated} />;
  }
  if (phase === "login" || !actor) {
    return <LoginPage onAuthenticated={authenticated} />;
  }

  const has = (permission: string) => actor.permissions.includes(permission);
  const dashboardAllowed = has("system.read") || has("modules.read") || has("jobs.read");
  const accountPage = <AccountPage actor={actor} onPasswordChanged={() => {setActor(undefined); setPhase("login");}} />;

  const moduleRoute = isModuleRoute(path);
  const allowedPath = moduleRoute ? path : accessiblePath(actor, path, moduleNavigation);
  let page;
  switch (allowedPath.split("#")[0]) {
    case "/account":
      page = accountPage;
      break;
    case "/files":
    case "/files/storage":
    case "/files/windows":
      page = has("security.self.read")
        ? (
          <FilesPage
            revision={revision}
            canManage={has("files.manage")}
            section={fileSection(allowedPath)}
            onSectionChange={(section) => navigate(fileSectionPath(section))}
          />
        )
        : accountPage;
      break;
    case "/system":
      page = has("system.read")
        ? (
          <SystemPage
            revision={revision}
            section={systemSection(allowedPath)}
            onSectionChange={(section) => navigate("/system#" + section)}
            canReadUpdates={has("updates.read")}
            canManageUpdates={has("updates.manage")}
            canManageStorage={has("storage.manage")}
            canReadNetwork={has("network.read")}
            canManageNetwork={has("network.manage")}
            canUseConsole={has("system.console")}
          />
        )
        : accountPage;
      break;
    case "/modules":
      page = has("modules.read")
        ? <ModulesPage revision={revision} canManage={has("modules.manage")} canReadJobs={has("jobs.read")} onNavigate={navigate} />
        : accountPage;
      break;
    case "/jobs":
      page = has("jobs.read") ? <JobsPage revision={revision} canManage={has("jobs.cancel")} selectedJob={allowedPath.split("#job=")[1]} /> : accountPage;
      break;
    case "/audit":
      page = has("audit.read") ? <AuditPage revision={revision} /> : accountPage;
      break;
    case "/users":
      page = has("security.users.read")
        ? <UsersPage revision={revision} canManage={has("security.users.manage")} currentUserID={actor.id} />
        : accountPage;
      break;
    default:
      page = moduleRoute ? (!moduleNavigation ? navigationError ? <div className="page"><ErrorNotice error={navigationError} /><button type="button" className="button secondary" onClick={() => window.dispatchEvent(new CustomEvent("home-ai-core:modules-changed"))}>{u("retry")}</button></div> : <LoadingState /> : <ModulePage path={path} modules={moduleNavigation}
        permitted={has("security.self.read")} canReadModules={has("modules.read")} onNavigate={navigate} />)
        : dashboardAllowed ? <Dashboard actor={actor} revision={revision} onNavigate={navigate} /> : accountPage;
  }


  return (
    <Shell actor={actor} path={allowedPath} realtime={realtime} availableUpdate={availableUpdate} modules={moduleNavigation} onNavigate={navigate} onLogout={logout} loggingOut={loggingOut}>
      {logoutError !== undefined && <ErrorNotice error={logoutError} message={u("logoutFailed")} />}
      {page}
    </Shell>
  );
}
