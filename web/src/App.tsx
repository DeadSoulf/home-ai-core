import { useEffect, useState } from "react";
import { api, APIError, connectRealtime, setCSRFToken, type RealtimeStatus } from "./api/client";
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

type Phase = "loading" | "setup" | "login" | "app";

function currentPath(): string {
  const path = window.location.pathname.replace(/\/+$/, "") || "/";
  const known = ["/", "/files", "/files/storage", "/files/windows", "/system", "/modules", "/jobs", "/audit", "/users", "/account"];
  const moduleRoute = /^\/modules\/[a-z][a-z0-9.-]*$/.test(path);
  if (!known.includes(path) && !moduleRoute) return "/";
  return path + (path === "/system" || path === "/files" ? window.location.hash : "");
}


export default function App() {
  const {t} = useI18n();
  const [phase, setPhase] = useState<Phase>("loading");
  const [actor, setActor] = useState<Actor>();
  const [path, setPath] = useState(currentPath);
  const [realtime, setRealtime] = useState<RealtimeStatus>("disconnected");
  const [revision, setRevision] = useState(0);
  const [availableUpdate, setAvailableUpdate] = useState<string>();
  const [moduleNavigation, setModuleNavigation] = useState<ModuleNavigationState[]>();

  useEffect(() => {
    api.setupStatus()
      .then(async (status) => {
        if (!status.initialized) {
          setPhase("setup");
          return;
        }
        try {
          const current = await api.me();
          setActor(current);
          setPhase("app");
        } catch (error) {
          if (error instanceof APIError && error.status === 401) {
            setPhase("login");
            return;
          }
          throw error;
        }
      })
      .catch(() => setPhase("login"));
  }, []);

  useEffect(() => {
    const pop = () => setPath(currentPath());
    window.addEventListener("popstate", pop);
    return () => window.removeEventListener("popstate", pop);
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
    const check = async () => {
      if (checking) return;
      checking = true;
      try {
        const result = await api.updateStatus();
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
      if (document.visibilityState === "visible") void check();
    };
    const updateStatusEvent = (event: Event) => {
      const detail = (event as CustomEvent<string | undefined>).detail;
      setAvailableUpdate(detail || undefined);
    };

    void check();
    const timer = window.setInterval(check, 5 * 60 * 1000);
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
      setModuleNavigation(undefined);
      return;
    }
    let stopped = false;
    const refresh = async () => {
      try {
        const items = await api.moduleNavigation();
        if (!stopped) setModuleNavigation(items);
      } catch {
        // Keep the previous navigation snapshot if the registry is temporarily unavailable.
      }
    };
    void refresh();
    const changed = () => { void refresh(); };
    window.addEventListener("home-ai-core:modules-changed", changed);
    return () => {
      stopped = true;
      window.removeEventListener("home-ai-core:modules-changed", changed);
    };
  }, [phase, actor, revision]);

  useEffect(() => {
    if (phase !== "app" || !actor || !moduleNavigation) return;
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
    try {
      await api.logout();
    } finally {
      setCSRFToken();
      setActor(undefined);
      setPhase("login");
    }
  };

  const navigate = (nextPath: string) => {
    const allowed = actor ? accessiblePath(actor, nextPath, moduleNavigation) : "/";
    if (allowed !== path) {
      window.history.pushState({}, "", allowed);
      setPath(allowed);
    }
  };

  if (phase === "loading") {
    return <div className="boot-screen"><div className="brand-mark">H</div><span>{t("starting")}</span></div>;
  }
  if (phase === "setup") {
    return <FirstRunPage onAuthenticated={authenticated} />;
  }
  if (phase === "login" || !actor) {
    return <LoginPage onAuthenticated={authenticated} />;
  }

  const has = (permission: string) => actor.permissions.includes(permission);
  const dashboardAllowed = has("system.read") || has("modules.read") || has("jobs.read");
  const accountPage = <AccountPage actor={actor} onPasswordChanged={() => {setActor(undefined); setPhase("login");}} />;

  const allowedPath = accessiblePath(actor, path, moduleNavigation);
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
          />
        )
        : accountPage;
      break;
    case "/modules":
      page = has("modules.read")
        ? <ModulesPage revision={revision} />
        : accountPage;
      break;
    case "/jobs":
      page = has("jobs.read") ? <JobsPage revision={revision} canManage={has("jobs.cancel")} /> : accountPage;
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
      page = dashboardAllowed ? <Dashboard actor={actor} revision={revision} onNavigate={navigate} /> : accountPage;
  }


  return (
    <Shell actor={actor} path={allowedPath} realtime={realtime} availableUpdate={availableUpdate} modules={moduleNavigation} onNavigate={navigate} onLogout={logout}>
      {page}
    </Shell>
  );
}
