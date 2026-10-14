import { useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { api } from "../api/client";
import type { Actor } from "../api/types";
import { COPYRIGHT_NOTICE } from "../branding";
import { LanguageSwitch, useI18n } from "../i18n";
import { ErrorNotice } from "../components/ErrorNotice";
import { validateAuth } from "../ui/authValidation";
import { useUI } from "../ui/useUI";
import type { UIKey } from "../ui/messages";

export function LoginPage(props: {onAuthenticated: (actor: Actor) => void}) { return <AuthPage {...props} setup={false} />; }
export function FirstRunPage(props: {onAuthenticated: (actor: Actor) => void}) { return <AuthPage {...props} setup />; }

function AuthPage({onAuthenticated, setup}: {onAuthenticated: (actor: Actor) => void; setup: boolean}) {
  const {t} = useI18n(); const u = useUI(); const prefix = useId();
  const [username, setUsername] = useState("owner"), [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("Home Owner"), [bootstrapToken, setBootstrapToken] = useState("");
  const [confirmation, setConfirmation] = useState(""), [visible, setVisible] = useState(false);
  const [error, setError] = useState<unknown>(), [errors, setErrors] = useState<Record<string, UIKey>>({});
  const [busy, setBusy] = useState(false); const submitting = useRef(false);
  const inputProps = (field: string, hint?: string) => ({
    id: prefix + field, name: field, "aria-invalid": Boolean(errors[field]),
    "aria-describedby": [errors[field] ? prefix + field + "-error" : "", hint ? prefix + hint : ""].filter(Boolean).join(" ") || undefined,
  });
  const fieldError = (field: string) => errors[field] && <span className="form-error" id={prefix + field + "-error"}>{u(errors[field])}</span>;
  async function submit(event: FormEvent) {
    event.preventDefault(); if (submitting.current) return;
    const next = validateAuth({username, password, displayName, bootstrapToken, confirmation}, setup);
    setErrors(next); setError(undefined);
    if (Object.keys(next).length) { document.getElementById(prefix + Object.keys(next)[0])?.focus(); return; }
    submitting.current = true; setBusy(true);
    try {
      const result = setup ? await api.bootstrap({bootstrapToken: bootstrapToken.trim(), username: username.trim(), displayName, password}) : await api.login(username.trim(), password);
      setPassword(""); setConfirmation(""); setBootstrapToken(""); onAuthenticated(result.actor);
    } catch (reason) { setError(reason); }
    finally { submitting.current = false; setBusy(false); }
  }
  return <AuthLayout title={t(setup ? "createOwner" : "signIn")} subtitle={t(setup ? "createOwnerSubtitle" : "signInSubtitle")}>
    <form onSubmit={submit} className="auth-form" noValidate aria-busy={busy}>
      {setup && <label htmlFor={prefix + "bootstrapToken"}>{t("setupCode")}
        <input {...inputProps("bootstrapToken", "setup-hint")} type="password" required value={bootstrapToken} disabled={busy}
          onChange={(e) => setBootstrapToken(e.target.value)} autoComplete="off" />
        <small id={prefix + "setup-hint"}>{t("setupCodeHint")}</small>{fieldError("bootstrapToken")}</label>}
      <label htmlFor={prefix + "username"}>{t("username")}
        <input {...inputProps("username", setup ? "username-hint" : undefined)} required minLength={3} maxLength={64}
          autoCapitalize="none" spellCheck={false} value={username} disabled={busy} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
        {setup && <small id={prefix + "username-hint"}>{u("usernameHint")}</small>}{fieldError("username")}</label>
      {setup && <label htmlFor={prefix + "displayName"}>{t("displayName")}
        <input {...inputProps("displayName")} value={displayName} disabled={busy} onChange={(e) => setDisplayName(e.target.value)} autoComplete="name" />{fieldError("displayName")}</label>}
      <label htmlFor={prefix + "password"}>{t("password")}
        <input {...inputProps("password", setup ? "password-hint" : undefined)} required type={visible ? "text" : "password"}
          value={password} disabled={busy} onChange={(e) => setPassword(e.target.value)} autoComplete={setup ? "new-password" : "current-password"} />
        {setup && <small id={prefix + "password-hint"}>{u("passwordHint")}</small>}{fieldError("password")}</label>
      {setup && <label htmlFor={prefix + "confirmation"}>{u("confirmPassword")}
        <input {...inputProps("confirmation")} required type={visible ? "text" : "password"} value={confirmation} disabled={busy}
          onChange={(e) => setConfirmation(e.target.value)} autoComplete="new-password" />{fieldError("confirmation")}</label>}
      <button type="button" className="button secondary password-toggle" aria-pressed={visible} aria-controls={prefix + "password"}
        onClick={() => setVisible((value) => !value)}>{u(visible ? "hidePassword" : "showPassword")}</button>
      {Object.keys(errors).length > 0 && <div className="sr-only" role="alert">{Object.values(errors).map(u).join(" ")}</div>}
      {error !== undefined && <ErrorNotice error={error} />}
      <button type="submit" className="button primary" disabled={busy}>{t(setup ? busy ? "initializing" : "initialize" : busy ? "signingIn" : "signIn")}</button>
    </form>
  </AuthLayout>;
}

function AuthLayout(props: {title: string; subtitle: string; children: ReactNode}) {
  const {t} = useI18n();
  return (
    <main className="auth-screen">
      <div className="auth-shell">

        <section className="auth-showcase" aria-hidden="true">
          <div className="auth-network-composition">
            <svg className="auth-network-art" viewBox="0 0 560 520" fill="none" focusable="false">
              <defs>
                <radialGradient id="auth-network-glow">
                  <stop stopColor="#367CE5" stopOpacity=".28" />
                  <stop offset="1" stopColor="#367CE5" stopOpacity="0" />
                </radialGradient>
                <linearGradient id="auth-network-case" x1="196" y1="148" x2="367" y2="395" gradientUnits="userSpaceOnUse">
                  <stop stopColor="#223C61" />
                  <stop offset="1" stopColor="#0D1828" />
                </linearGradient>
                <linearGradient id="auth-network-node" x1="0" y1="-36" x2="0" y2="36" gradientUnits="userSpaceOnUse">
                  <stop stopColor="#233E60" />
                  <stop offset="1" stopColor="#112035" />
                </linearGradient>
                <linearGradient id="auth-network-accent" x1="210" y1="190" x2="348" y2="360" gradientUnits="userSpaceOnUse">
                  <stop stopColor="#7DCAFF" />
                  <stop offset="1" stopColor="#4779F8" />
                </linearGradient>
              </defs>
              <ellipse cx="280" cy="262" rx="255" ry="228" fill="url(#auth-network-glow)" />
              <g className="auth-network-rings">
                <circle cx="280" cy="262" r="210" />
                <circle cx="280" cy="262" r="168" strokeDasharray="3 13" />
                <ellipse cx="280" cy="262" rx="244" ry="125" transform="rotate(-24 280 262)" />
              </g>
              <g className="auth-network-links">
                <path d="M280 158V108" />
                <path d="M194 272H129" />
                <path d="M366 272H431" />
                <path d="M280 386V415" />
              </g>
              <g className="auth-network-nodes">
                <g transform="translate(280 74)">
                  <circle r="36" fill="url(#auth-network-node)" stroke="#609BE4" strokeOpacity=".55" />
                  <circle r="28" stroke="#8CB7FF" strokeOpacity=".11" />
                  <path d="M-15 -1Q0 -15 15 -1M-10 6Q0 -3 10 6M-4 12Q0 8 4 12" stroke="#9BCAFF" strokeWidth="2.6" strokeLinecap="round" />
                  <circle cy="18" r="2.5" fill="#8DC8FF" />
                </g>
                <g transform="translate(92 272)">
                  <circle r="36" fill="url(#auth-network-node)" stroke="#609BE4" strokeOpacity=".55" />
                  <rect x="-17" y="-12" width="34" height="24" rx="4" stroke="#9BCAFF" strokeWidth="2.4" />
                  <path d="M-23 19H23L17 12H-17L-23 19Z" stroke="#9BCAFF" strokeWidth="2.3" strokeLinejoin="round" />
                </g>
                <g transform="translate(468 272)">
                  <circle r="36" fill="url(#auth-network-node)" stroke="#609BE4" strokeOpacity=".55" />
                  <path d="M0 -21 18 -13V1C18 13 10 21 0 25-10 21-18 13-18 1V-13L0 -21Z" stroke="#9BCAFF" strokeWidth="2.4" strokeLinejoin="round" />
                  <path d="M-8 1 -2 7 10 -7" stroke="#9BCAFF" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" />
                </g>
                <g transform="translate(280 450)">
                  <circle r="36" fill="url(#auth-network-node)" stroke="#609BE4" strokeOpacity=".55" />
                  <circle cx="-13" cy="-9" r="5" stroke="#9BCAFF" strokeWidth="2.2" />
                  <circle cx="14" cy="-9" r="5" stroke="#9BCAFF" strokeWidth="2.2" />
                  <circle cy="15" r="5" stroke="#9BCAFF" strokeWidth="2.2" />
                  <path d="M-8 -5 -3 10M9 -5 3 10M-8 -9H9" stroke="#9BCAFF" strokeWidth="2.2" strokeLinecap="round" />
                </g>
              </g>
              <g className="auth-network-server">
                <rect x="192" y="152" width="176" height="234" rx="26" fill="url(#auth-network-case)" stroke="#6BA9FC" strokeOpacity=".6" strokeWidth="1.6" />
                <rect x="204" y="165" width="152" height="208" rx="17" stroke="#9CCAFF" strokeOpacity=".13" />
                <rect x="217" y="189" width="126" height="48" rx="10" fill="#101F32" stroke="#77ADFA" strokeOpacity=".4" />
                <rect x="217" y="247" width="126" height="48" rx="10" fill="#101F32" stroke="#77ADFA" strokeOpacity=".4" />
                <rect x="217" y="305" width="126" height="48" rx="10" fill="#101F32" stroke="#77ADFA" strokeOpacity=".4" />
                <g stroke="url(#auth-network-accent)" strokeWidth="3.3" strokeLinecap="round">
                  <path d="M235 211H272M235 269H272M235 327H272" />
                </g>
                <g fill="#85BDFF">
                  <circle cx="313" cy="213" r="3.5" />
                  <circle cx="327" cy="213" r="3.5" opacity=".5" />
                  <circle cx="313" cy="271" r="3.5" />
                  <circle cx="327" cy="271" r="3.5" opacity=".5" />
                  <circle cx="313" cy="329" r="3.5" />
                  <circle cx="327" cy="329" r="3.5" opacity=".5" />
                </g>
                <path d="M265 170H295" stroke="#9CCAFF" strokeWidth="2.6" strokeLinecap="round" strokeOpacity=".7" />
                <circle cx="280" cy="152" r="5" fill="#9FD5FF" />
              </g>
            </svg>
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
