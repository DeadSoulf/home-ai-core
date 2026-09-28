import { useState, type FormEvent, type ReactNode } from "react";
import { api, APIError } from "../api/client";
import type { Actor } from "../api/types";

function messageFor(error: unknown): string {
  if (error instanceof APIError && error.code === "bootstrap_local_only") {
    return "First-owner setup is restricted to localhost. Use an SSH tunnel to 127.0.0.1:8080 and open the UI through that tunnel.";
  }
  return error instanceof Error ? error.message : "Request failed";
}

export function LoginPage({onAuthenticated}: {onAuthenticated: (actor: Actor) => void}) {
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
    <AuthLayout title="Sign in" subtitle="Manage your private home infrastructure.">
      <form onSubmit={submit} className="auth-form">
        <label>
          Username
          <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
        </label>
        <label>
          Password
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        </label>
        {error && <div className="form-error">{error}</div>}
        <button className="button primary" disabled={busy}>
          {busy ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </AuthLayout>
  );
}

export function FirstRunPage({onAuthenticated}: {onAuthenticated: (actor: Actor) => void}) {
  const [bootstrapToken, setBootstrapToken] = useState("");
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
        bootstrapToken,
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
    <AuthLayout title="Create the first owner" subtitle="Initialize this Home-AI-Core node.">
      <div className="notice">
        First setup is intentionally local-only. Read the one-time bootstrap token from the server state directory and access this page through localhost, for example via an SSH tunnel.
      </div>
      <form onSubmit={submit} className="auth-form">
        <label>
          Bootstrap token
          <input value={bootstrapToken} onChange={(e) => setBootstrapToken(e.target.value)} autoComplete="off" />
        </label>
        <label>
          Username
          <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
        </label>
        <label>
          Display name
          <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        </label>
        <label>
          Password
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
        </label>
        {error && <div className="form-error">{error}</div>}
        <button className="button primary" disabled={busy}>
          {busy ? "Initializing…" : "Initialize Home-AI-Core"}
        </button>
      </form>
    </AuthLayout>
  );
}

function AuthLayout(props: {title: string; subtitle: string; children: ReactNode}) {
  return (
    <main className="auth-screen">
      <section className="auth-card">
        <div className="brand auth-brand">
          <div className="brand-mark">H</div>
          <div>
            <strong>Home-AI-Core</strong>
            <span>Private home infrastructure</span>
          </div>
        </div>
        <h1>{props.title}</h1>
        <p className="muted">{props.subtitle}</p>
        {props.children}
      </section>
    </main>
  );
}
