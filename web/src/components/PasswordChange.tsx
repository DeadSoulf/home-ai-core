import {useState, type FormEvent} from "react";
import {api} from "../api/client";
import {useI18n} from "../i18n";

export function PasswordChange({onChanged}: {onChanged: () => void}) {
  const {t} = useI18n();
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function save(event: FormEvent) {
    event.preventDefault(); setError("");
    if (password !== confirmation) {setError(t("passwordMismatch")); return;}
    setBusy(true);
    try {await api.changePassword(current, password); setCurrent(""); setPassword(""); setConfirmation(""); onChanged();}
    catch (reason) {setError(reason instanceof Error ? reason.message : t("requestFailed"));}
    finally {setBusy(false);}
  }
  return <form className="user-form" onSubmit={save}>
    <label>{t("currentPassword")}<input type="password" autoComplete="current-password" required value={current} onChange={(event) => setCurrent(event.target.value)}/></label>
    <label>{t("newPassword")}<input type="password" autoComplete="new-password" minLength={12} maxLength={256} required value={password} onChange={(event) => setPassword(event.target.value)}/></label>
    <label>{t("confirmPassword")}<input type="password" autoComplete="new-password" minLength={12} maxLength={256} required value={confirmation} onChange={(event) => setConfirmation(event.target.value)}/></label>
    <p className="muted small">{t("passwordSessionsNotice")}</p>
    {error && <div role="alert" className="form-error">{error}</div>}
    <button className="button primary" disabled={busy}>{busy ? t("working") : t("changePassword")}</button>
  </form>;
}

