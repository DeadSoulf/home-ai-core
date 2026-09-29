import { useEffect, useState } from "react";
import { api, APIError, connectRealtime, setCSRFToken, type RealtimeStatus } from "./api/client";
import type { Actor } from "./api/types";
import { Shell } from "./components/Shell";
import { useI18n } from "./i18n";
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
  return ["/", "/files", "/system", "/modules", "/jobs", "/audit", "/users"].includes(path) ? path : "/";
}

export default function App() {
  const {t} = useI18n();
  const [phase, setPhase] = useState<Phase>("loading");
  const [actor, setActor] = useState<Actor>();
  const [path, setPath] = useState(currentPath);
  const [realtime, setRealtime] = useState<RealtimeStatus>("disconnected");
  const [revision, setRevision] = useState(0);
  const [availableUpdate, setAvailableUpdate] = useState<string>();

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
    if (nextPath !== path) {
      window.history.pushState({}, "", nextPath);
      setPath(nextPath);
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
  const dashboardAllowed = has("system.read") && has("modules.read") && has("jobs.read");
  const accountPage = <AccountPage actor={actor} />;

  let page;
  switch (path) {
    case "/files":
      page = has("security.self.read")
        ? <FilesPage revision={revision} canManage={has("files.manage")} />
        : accountPage;
      break;
    case "/system":
      page = has("system.read")
        ? (
          <SystemPage
            revision={revision}
            canReadNetwork={has("network.read")}
            canManageNetwork={has("network.manage")}
          />
        )
        : accountPage;
      break;
    case "/modules":
      page = has("modules.read") ? <ModulesPage revision={revision} /> : accountPage;
      break;
    case "/jobs":
      page = has("jobs.read") ? <JobsPage revision={revision} /> : accountPage;
      break;
    case "/audit":
      page = has("audit.read") ? <AuditPage revision={revision} /> : accountPage;
      break;
    case "/users":
      page = has("security.users.read")
        ? <UsersPage revision={revision} canManage={has("security.users.manage")} />
        : accountPage;
      break;
    default:
      page = dashboardAllowed ? <Dashboard revision={revision} /> : accountPage;
  }

  return (
    <Shell actor={actor} path={path} realtime={realtime} availableUpdate={availableUpdate} onNavigate={navigate} onLogout={logout}>
      {page}
    </Shell>
  );
}
