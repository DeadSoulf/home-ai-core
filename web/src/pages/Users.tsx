import { useCallback, useMemo, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type {
  AccessCatalog,
  PermissionScope,
  UserAccount,
  UserProfile,
} from "../api/types";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useResource } from "../hooks/useResource";
import { useI18n, type StringKey } from "../i18n";
import { PageHeading, Status } from "./Dashboard";

const baselinePermissions = new Set([
  "security.self.read",
  "security.sessions.manage",
]);

const administratorOnlyPermissions = new Set([
  "security.users.manage",
  "security.roles.manage",
]);

export function UsersPage({revision, canManage, currentUserID}: {revision: number; canManage: boolean; currentUserID?: string}) {
  const {t, date} = useI18n();
  const load = useCallback(async () => {
    const users = await api.users();
    const access = canManage ? await api.userAccessCatalog() : undefined;
    const quotas = canManage ? await api.fileUserQuotas().catch(() => []) : [];
    return {users, access, quotas};
  }, [canManage]);
  const resource = useResource(load, revision);

  const [selectedUserID, setSelectedUserID] = useState("");
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [profile, setProfile] = useState<UserProfile>("friend");
  const [permissions, setPermissions] = useState<string[]>([]);
  const [resourcePermissions, setResourcePermissions] = useState<PermissionScope[]>([]);
  const [disabled, setDisabled] = useState(false);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState("");
  const [notice, setNotice] = useState("");

  const users = resource.data?.users || [];
  const catalog = resource.data?.access;
  const selectedUser = users.find((user) => user.id === selectedUserID);

  const permissionGroups = useMemo(() => {
    const groups = new Map<string, AccessCatalog["permissions"]>();
    for (const permission of catalog?.permissions || []) {
      const values = groups.get(permission.category) || [];
      values.push(permission);
      groups.set(permission.category, values);
    }
    return [...groups.entries()];
  }, [catalog]);

  function profileTemplate(nextProfile: UserProfile) {
    return catalog?.profiles.find((item) => item.id === nextProfile);
  }

  function applyProfile(nextProfile: UserProfile) {
    setProfile(nextProfile);
    const template = profileTemplate(nextProfile);
    setPermissions(template?.default_permissions || []);
    setFormError("");
    setNotice(t("userProfileTemplateApplied"));
  }

  function resetEditor() {
    setSelectedUserID("");
    setUsername("");
    setDisplayName("");
    setPassword("");
    setProfile("friend");
    setPermissions(profileTemplate("friend")?.default_permissions || []);
    setResourcePermissions([]);
    setDisabled(false);
    setFormError("");
    setNotice("");
  }

  function editUser(user: UserAccount) {
    setSelectedUserID(user.id);
    setUsername(user.username);
    setDisplayName(user.display_name);
    setPassword("");
    setProfile(user.profile || "friend");
    setPermissions(
      user.profile === "administrator"
        ? (user.permissions || [])
        : (user.permissions || []).filter((permission) => !administratorOnlyPermissions.has(permission)),
    );
    setResourcePermissions(user.resource_permissions || []);
    setDisabled(user.disabled);
    setFormError("");
    setNotice("");
  }

  function togglePermission(permission: string) {
    if (profile === "administrator" || baselinePermissions.has(permission)) return;
    setPermissions((current) =>
      current.includes(permission)
        ? current.filter((item) => item !== permission)
        : [...current, permission].sort(),
    );
  }

  function hasResourcePermission(resourceType: string, resourceID: string, permission: string) {
    return resourcePermissions.some((item) =>
      item.resource_type === resourceType &&
      item.resource_id === resourceID &&
      item.permission === permission
    );
  }

  function toggleResourcePermission(resourceType: string, resourceID: string, permission: string) {
    if (profile === "administrator") return;
    const exists = hasResourcePermission(resourceType, resourceID, permission);
    setResourcePermissions((current) => {
      if (exists) {
        return current.filter((item) => !(
          item.resource_type === resourceType &&
          item.resource_id === resourceID &&
          item.permission === permission
        ));
      }
      return [...current, {
        resource_type: resourceType,
        resource_id: resourceID,
        permission,
      }];
    });
  }

  async function saveIdentity(event: FormEvent) {
    event.preventDefault(); if (!selectedUser) return;
    setSaving(true); setFormError(""); setNotice("");
    try {
      await api.updateUserIdentity(selectedUser.id, username, displayName);
      setNotice(t("userIdentitySaved")); resource.reload();
      window.dispatchEvent(new CustomEvent("home-ai-core:user-access-changed"));
    } catch (reason) {setFormError(reason instanceof Error ? reason.message : t("requestFailed"));}
    finally {setSaving(false);}
  }
  async function resetPassword(event: FormEvent) {
    event.preventDefault(); if (!selectedUser) return;
    setSaving(true); setFormError(""); setNotice("");
    try {await api.resetUserPassword(selectedUser.id, password); setPassword(""); setNotice(t("userPasswordReset"));}
    catch (reason) {setFormError(reason instanceof Error ? reason.message : t("requestFailed"));}
    finally {setSaving(false);}
  }
  async function saveQuota(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!selectedUser) return;
    const value = new FormData(event.currentTarget).get("quota_gib");
    setSaving(true); setFormError(""); setNotice("");
    try {
      const result = await api.updateUserQuota(selectedUser.id, value ? Math.floor(Number(value) * 2 ** 30) : 0);
      setNotice(result.warning || t("quotaSaved")); resource.reload();
    } catch (reason) {setFormError(reason instanceof Error ? reason.message : t("requestFailed"));}
    finally {setSaving(false);}
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!catalog) return;
    setSaving(true);
    setFormError("");
    setNotice("");
    try {
      if (selectedUser) {
        const updated = await api.updateUserAccess(selectedUser.id, {
          profile,
          permissions,
          resourcePermissions,
          disabled,
        });
        setNotice(updated.warning || t("userAccessSaved"));
      } else {
        await api.createUser({
          username,
          displayName,
          password,
          profile,
          permissions,
          resourcePermissions,
        });
        setNotice(t("userCreated"));
      }
      resource.reload();
      if (!selectedUser) {
        setUsername("");
        setDisplayName("");
        setPassword("");
      }
      window.dispatchEvent(new CustomEvent("home-ai-core:user-access-changed"));
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setSaving(false);
    }
  }

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  return (
    <div className="page">
      <PageHeading title={t("users")} subtitle={t("usersSubtitle")} />
      {formError && <div role="alert" className="form-error">{formError}</div>}
      {notice && <div role="status" className="notice">{notice}</div>}

      <Panel title={t("userAccounts")} className="wide">
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("displayName")}</th>
                <th>{t("username")}</th>
                <th>{t("userProfile")}</th>
                <th>{t("userCapabilities")}</th>
                <th>{t("userResourceAccess")}</th>
                <th>{t("status")}</th>
                <th>{t("lastLogin")}</th>
                {canManage && <th>{t("actions")}</th>}
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td><strong>{user.display_name}</strong></td>
                  <td className="mono">{user.username}</td>
                  <td>{profileLabel(user.profile, t)}</td>
                  <td>{user.profile === "administrator" ? t("userFullAccess") : user.permissions.length}</td>
                  <td>{user.profile === "administrator" ? t("userAllResources") : (user.resource_permissions?.length || 0)}</td>
                  <td><Status value={user.disabled ? "disabled" : "active"} /></td>
                  <td>{user.last_login_at ? date(user.last_login_at) : "—"}</td>
                  {canManage && (
                    <td>
                      <button
                        type="button"
                        className="button compact secondary"
                        onClick={() => editUser(user)}
                      >
                        {t("userEditAccess")}
                      </button>
                    </td>
                  )}
                </tr>
              ))}
              {users.length === 0 && (
                <tr><td colSpan={canManage ? 8 : 7} className="muted">{t("noUsers")}</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </Panel>

      {canManage && selectedUser && <Panel title={selectedUser.display_name} className="wide">
        <form className="user-form" onSubmit={saveIdentity}>
          <label>{t("username")}<input required minLength={3} maxLength={64} value={username} onChange={(event) => setUsername(event.target.value)}/></label>
          <label>{t("displayName")}<input required maxLength={128} value={displayName} onChange={(event) => setDisplayName(event.target.value)}/></label>
          <button className="button secondary" disabled={saving}>{t("saveIdentity")}</button>
        </form>
        {selectedUser.id !== currentUserID && <details><summary>{t("resetUserPassword")}</summary>
          <form className="user-form" onSubmit={resetPassword}>
            <label>{t("newPassword")}<input type="password" autoComplete="new-password" required minLength={12} maxLength={256} value={password} onChange={(event) => setPassword(event.target.value)}/></label>
            <p className="muted small">{t("passwordSessionsNotice")}</p>
            <button className="button secondary" disabled={saving}>{t("resetUserPassword")}</button>
          </form>
        </details>}
        <details><summary>{t("personalQuota")}</summary>
          <p className="muted small">{t("personalQuotaNotice")}</p>
          <form key={`${selectedUser.id}-${resource.data?.quotas.find((item) => item.user_id === selectedUser.id)?.quota_bytes}`} className="user-form" onSubmit={saveQuota}>
            <label>{t("quotaGiB")}<input name="quota_gib" type="number" min={0} step="any" placeholder={t("quotaUnlimited")} defaultValue={(resource.data?.quotas.find((item) => item.user_id === selectedUser.id)?.quota_bytes || 0) / 2 ** 30 || ""}/></label>
            <p className="muted small">{t("quotaAccountingNotice")}</p>
            <button className="button secondary" disabled={saving}>{t("save")}</button>
          </form>
        </details>
      </Panel>}

      {canManage && catalog && (
        <Panel
          title={selectedUser ? t("userEditProfile") : t("addUser")}
          className="wide"
        >
          <form className="user-access-form" onSubmit={submit}>
            <div className="user-access-top">
              {!selectedUser && (
                <>
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
                </>
              )}
              {selectedUser && (
                <div className="user-edit-identity">
                  <strong>{selectedUser.display_name}</strong>
                  <span className="mono">{selectedUser.username}</span>
                </div>
              )}
              <label>
                {t("userProfile")}
                <select
                  value={profile}
                  onChange={(event) => applyProfile(event.target.value as UserProfile)}
                >
                  {catalog.profiles.map((item) => (
                    <option key={item.id} value={item.id}>
                      {profileLabel(item.id, t)}
                    </option>
                  ))}
                </select>
              </label>
              {selectedUser && (
                <label className="user-access-inline-check">
                  <input
                    type="checkbox"
                    checked={disabled}
                    onChange={(event) => setDisabled(event.target.checked)}
                  />
                  <span>{t("userDisableAccount")}</span>
                </label>
              )}
            </div>

            <div className="notice">
              {profile === "administrator"
                ? t("userAdministratorNotice")
                : t("userCustomAccessNotice")}
            </div>

            <details className="user-access-section">
              <summary>{t("userCapabilities")}</summary>
              <div className="user-access-section-head">
                <div>
                  <h3>{t("userCapabilities")}</h3>
                  <p className="muted small">{t("userCapabilitiesNotice")}</p>
                </div>
                {profile !== "administrator" && (
                  <button
                    type="button"
                    className="button compact secondary"
                    onClick={() => setPermissions(profileTemplate(profile)?.default_permissions || [])}
                  >
                    {t("userApplyProfileTemplate")}
                  </button>
                )}
              </div>

              <div className="permission-groups">
                {permissionGroups.map(([category, values]) => (
                  <fieldset key={category} className="permission-group">
                    <legend>{categoryLabel(category, t)}</legend>
                    {values.map((permission) => {
                      const administratorOnly = administratorOnlyPermissions.has(permission.name);
                      const locked =
                        profile === "administrator" ||
                        baselinePermissions.has(permission.name) ||
                        administratorOnly;
                      const checked =
                        profile === "administrator" ||
                        baselinePermissions.has(permission.name) ||
                        (!administratorOnly && permissions.includes(permission.name));
                      return (
                        <label key={permission.name} className="permission-row">
                          <input
                            type="checkbox"
                            checked={checked}
                            disabled={locked}
                            onChange={() => togglePermission(permission.name)}
                          />
                          <span>
                            <strong>{permission.name}</strong>
                            <small>{permission.description}</small>
                          </span>
                        </label>
                      );
                    })}
                  </fieldset>
                ))}
              </div>
            </details>

            <section className="user-access-section">
              <h3>{t("userResourceAccess")}</h3>
              <p className="muted small">{t("userResourceAccessNotice")}</p>
              <div className="resource-access-list">
                {catalog.resources.map((item) => (
                  <div key={`${item.type}:${item.id}`} className="resource-access-card">
                    <div>
                      <strong>{item.name}</strong>
                      <div className="muted small">{item.description || item.type}</div>
                    </div>
                    <div className="resource-access-permissions">
                      {item.permissions.map((permission) => (
                        <label key={permission}>
                          <input
                            type="checkbox"
                            checked={profile === "administrator" || item.owner_user_id === selectedUserID || hasResourcePermission(item.type, item.id, permission)}
                            disabled={profile === "administrator" || item.owner_user_id === selectedUserID}
                            onChange={() => toggleResourcePermission(item.type, item.id, permission)}
                          />
                          <span>{resourcePermissionLabel(permission, t)}</span>
                        </label>
                      ))}
                    </div>
                  </div>
                ))}
                {catalog.resources.length === 0 && (
                  <div className="muted">{t("userNoResourcesYet")}</div>
                )}
              </div>
            </section>



            <div className="user-access-actions">
              {selectedUser && (
                <button type="button" className="button secondary" onClick={resetEditor}>
                  {t("cancel")}
                </button>
              )}
              <button className="button primary" type="submit" disabled={saving}>
                {saving
                  ? t("working")
                  : selectedUser
                    ? t("userSaveAccess")
                    : t("createUser")}
              </button>
            </div>
          </form>
        </Panel>
      )}
    </div>
  );
}

function profileLabel(profile: UserProfile, t: (key: StringKey) => string) {
  switch (profile) {
    case "administrator": return t("userProfileAdministrator");
    case "parent": return t("userProfileParent");
    case "child": return t("userProfileChild");
    case "guest": return t("userProfileGuest");
    case "friend": return t("userProfileFriend");
  }
}

function categoryLabel(category: string, t: (key: StringKey) => string) {
  switch (category) {
    case "system": return t("userCategorySystem");
    case "events": return t("userCategoryEvents");
    case "security": return t("userCategorySecurity");
    case "audit": return t("userCategoryAudit");
    case "jobs": return t("userCategoryJobs");
    case "modules": return t("userCategoryModules");
    case "updates": return t("userCategoryUpdates");
    case "storage": return t("userCategoryStorage");
    case "network": return t("userCategoryNetwork");
    case "files": return t("userCategoryFiles");
    default: return category;
  }
}

function resourcePermissionLabel(permission: string, t: (key: StringKey) => string) {
  switch (permission) {
    case "files.read": return t("userResourceRead");
    case "files.write": return t("userResourceWrite");
    default: return permission;
  }
}
