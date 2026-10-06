import { useState, type FormEvent, type ReactNode } from "react";
import { api, APIError } from "../api/client";
import type { Actor } from "../api/types";
import { COPYRIGHT_NOTICE } from "../branding";
import { LanguageSwitch, useI18n } from "../i18n";


export function LoginPage({onAuthenticated}: {onAuthenticated: (actor: Actor) => void}) {
  const {t} = useI18n();
  const messageFor = (error: unknown) => error instanceof APIError && error.code === "setup_network_only" ? t("setupNetworkOnly") : error instanceof Error ? error.message : t("requestFailed");
  const [username, setUsername] = useState("owner");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await api.login(username, password);
      onAuthenticated(result.actor);
    } catch (reason) {
      setError(messageFor(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthLayout title={t("signIn")} subtitle={t("signInSubtitle")}>
      <form onSubmit={submit} className="auth-form">
        <label>
          {t("username")}
          <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
        </label>
        <label>
          {t("password")}
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        </label>
        {error && <div className="form-error">{error}</div>}
        <button className="button primary" disabled={busy}>
          {busy ? t("signingIn") : t("signIn")}
        </button>
      </form>
    </AuthLayout>
  );
}

export function FirstRunPage({onAuthenticated}: {onAuthenticated: (actor: Actor) => void}) {
  const {t} = useI18n();
  const messageFor = (error: unknown) => error instanceof APIError && error.code === "setup_network_only" ? t("setupNetworkOnly") : error instanceof Error ? error.message : t("requestFailed");
  const [username, setUsername] = useState("owner");
  const [displayName, setDisplayName] = useState("Home Owner");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await api.bootstrap({
        username,
        displayName,
        password,
      });
      onAuthenticated(result.actor);
    } catch (reason) {
      setError(messageFor(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthLayout title={t("createOwner")} subtitle={t("createOwnerSubtitle")}>
      <form onSubmit={submit} className="auth-form">
        <label>
          {t("username")}
          <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
        </label>
        <label>
          {t("displayName")}
          <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        </label>
        <label>
          {t("password")}
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
        </label>
        {error && <div className="form-error">{error}</div>}
        <button className="button primary" disabled={busy}>
          {busy ? t("initializing") : t("initialize")}
        </button>
      </form>
    </AuthLayout>
  );
}

function AuthLayout(props: {title: string; subtitle: string; children: ReactNode}) {
  const {t} = useI18n();
  return (
    <main className="auth-screen">
      <div className="auth-shell">
        <section className="auth-showcase" aria-hidden="true">
          <div className="auth-showcase-orbit auth-showcase-orbit-one" />
          <div className="auth-showcase-orbit auth-showcase-orbit-two" />
          <div className="auth-showcase-content">
            <img className="auth-showcase-logo" src="/brand/variants/logo-stacked.webp" alt="" />
            <div className="auth-showcase-copy">
              <span className="auth-kicker">HOME AI CORE</span>
              <strong>Home-AI-Core</strong>
              <span>{t("privateInfrastructure")}</span>
            </div>
          </div>
        </section>

        <section className="auth-card">
          <div className="auth-toolbar"><LanguageSwitch /></div>
          <div className="auth-card-brand">
            <img className="auth-logo" src="/brand/app-icon.webp" alt="Home AI Core" />
            <div>
              <strong>Home-AI-Core</strong>
              <span>{t("privateInfrastructure")}</span>
            </div>
          </div>
          <h1>{props.title}</h1>
          <p className="muted">{props.subtitle}</p>
          {props.children}
          <div className="auth-copyright">{COPYRIGHT_NOTICE}</div>
        </section>
      </div>
    </main>
  );
}
