import { useCallback, useState, type FormEvent } from "react";
import { api } from "../api/client";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { PageHeading, Status } from "./Dashboard";

export function UsersPage({revision, canManage}: {revision: number; canManage: boolean}) {
  const {t, date} = useI18n();
  const load = useCallback(() => api.users(), []);
  const resource = useResource(load, revision);
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [creating, setCreating] = useState(false);
  const [formError, setFormError] = useState("");
  const [createdNotice, setCreatedNotice] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setCreating(true);
    setFormError("");
    setCreatedNotice("");
    try {
      const user = await api.createUser({username, displayName, password});
      setUsername("");
      setDisplayName("");
      setPassword("");
      setCreatedNotice(t("userCreated"));
      resource.reload();
      window.dispatchEvent(new CustomEvent("home-ai-core:user-created", {detail: user.id}));
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setCreating(false);
    }
  }

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  const users = resource.data || [];

  return (
    <div className="page">
      <PageHeading title={t("users")} subtitle={t("usersSubtitle")} />
      <div className={canManage ? "two-column" : ""}>
        <Panel title={t("userAccounts")}>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("displayName")}</th>
                  <th>{t("username")}</th>
                  <th>{t("roles")}</th>
                  <th>{t("status")}</th>
                  <th>{t("created")}</th>
                  <th>{t("lastLogin")}</th>
                </tr>
              </thead>
              <tbody>
                {users.map((user) => (
                  <tr key={user.id}>
                    <td>{user.display_name}</td>
                    <td className="mono">{user.username}</td>
                    <td>{user.roles.join(", ") || "—"}</td>
                    <td><Status value={user.disabled ? "disabled" : "active"} /></td>
                    <td>{date(user.created_at)}</td>
                    <td>{user.last_login_at ? date(user.last_login_at) : "—"}</td>
                  </tr>
                ))}
                {users.length === 0 && (
                  <tr><td colSpan={6} className="muted">{t("noUsers")}</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </Panel>

        {canManage && (
          <Panel title={t("addUser")}>
            <form className="user-form" onSubmit={submit}>
              <label>
                {t("username")}
                <input
                  required
                  minLength={3}
                  maxLength={64}
                  autoComplete="off"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                />
              </label>
              <label>
                {t("displayName")}
                <input
                  required
                  maxLength={128}
                  autoComplete="off"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                />
              </label>
              <label>
                {t("password")}
                <input
                  required
                  minLength={12}
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </label>
              <p className="muted small">{t("memberPermissionNotice")}</p>
              {formError && <div className="form-error">{formError}</div>}
              {createdNotice && <div className="storage-success">{createdNotice}</div>}
              <button className="button primary" type="submit" disabled={creating}>
                {creating ? t("creatingUser") : t("createUser")}
              </button>
            </form>
          </Panel>
        )}
      </div>
    </div>
  );
}
