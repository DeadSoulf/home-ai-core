import { useEffect, useState } from "react";
import { api, APIError, connectRealtime, setCSRFToken, type RealtimeStatus } from "./api/client";
import type { Actor } from "./api/types";
import { Shell } from "./components/Shell";
import { useI18n } from "./i18n";
import { FirstRunPage, LoginPage } from "./pages/Auth";
import { AuditPage } from "./pages/Audit";
import { Dashboard } from "./pages/Dashboard";
import { JobsPage } from "./pages/Jobs";
import { ModulesPage } from "./pages/Modules";
import { SystemPage } from "./pages/System";
import { UpdatesPage } from "./pages/Updates";

type Phase = "loading" | "setup" | "login" | "app";

function currentPath(): string {
  const path = window.location.pathname.replace(/\/+$/, "") || "/";
  return ["/", "/system", "/modules", "/updates", "/jobs", "/audit"].includes(path) ? path : "/";
}

export default function App() {
  const {t} = useI18n();
  const [phase, setPhase] = useState<Phase>("loading");
  const [actor, setActor] = useState<Actor>();
  const [path, setPath] = useState(currentPath);
  const [realtime, setRealtime] = useState<RealtimeStatus>("disconnected");
  const [revision, setRevision] = useState(0);

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
    if (phase !== "app") {
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
  }, [phase]);

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

  let page;
  switch (path) {
    case "/system":
      page = <SystemPage revision={revision} />;
      break;
    case "/modules":
      page = <ModulesPage revision={revision} />;
      break;
    case "/updates":
      page = <UpdatesPage revision={revision} />;
      break;
    case "/jobs":
      page = <JobsPage revision={revision} />;
      break;
    case "/audit":
      page = <AuditPage revision={revision} />;
      break;
    default:
      page = <Dashboard revision={revision} />;
  }

  return (
    <Shell actor={actor} path={path} realtime={realtime} onNavigate={navigate} onLogout={logout}>
      {page}
    </Shell>
  );
}
