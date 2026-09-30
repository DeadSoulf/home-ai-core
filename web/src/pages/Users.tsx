import { useCallback, useMemo, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { PermissionScope, UserAccount, UserProfileName } from "../api/types";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { PageHeading, Status } from "./Dashboard";

export function UsersPage({revision, canManage}: {revision: number; canManage: boolean}) {
  const {t, date} = useI18n();
  const load = useCallback(async () => {
    const users = await api.users();
    const catalog = canManage ? await api.userAccessCatalog() : undefined;
    return {users, catalog};
  }, [canManage]);
  const resource = useResource(load, revision);

  const [editingUserID, setEditingUserID] = useState("");
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [profile, setProfile] = useState<UserProfileName>("parent");
  const [disabled, setDisabled] = useState(false);
  const [permissions, setPermissions] = useState<string[]>([]);
  const [resourcePermissions, setResourcePermissions] = useState<PermissionScope[]>([]);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState("");
  const [notice, setNotice] = useState("");

  const users = resource.data?.users || [];
  const catalog = resource.data?.catalog;
  const editingUser = users.find((user) => user.id === editingUserID);
  const permissionDefinitions = catalog?.access.permissions || [];
  const resources = catalog?.resources || [];

  const profileTemplates = useMemo(() => {
    const profiles = catalog?.access.profiles || [];
    return profiles.filter((item) => !item.legacy || editingUser?.profile === item.name);
  }, [catalog, editingUser?.profile]);

  const groupedPermissions = useMemo(() => {
    const groups = new Map<string, typeof permissionDefinitions>();
    for (const permission of permissionDefinitions) {
      const group = permission.name.split(".")[0] || "other";
      const current = groups.get(group) || [];
      current.push(permission);
      groups.set(group, current);
    }
    return [...groups.entries()];
  }, [permissionDefinitions]);

  function profileLabel(value: UserProfileName) {
    switch (value) {
      case "administrator": return t("userProfileAdministrator");
      case "parent": return t("userProfileParent");
      case "child": return t("userProfileChild");
      case "friend": return t("userProfileFriend");
      case "guest": return t("userProfileGuest");
      case "member": return t("userProfileLegacyMember");
    }
  }

  function permissionGroupLabel(group: string) {
    switch (group) {
      case "system": return t("permissionGroupSystem");
      case "events": return t("permissionGroupEvents");
      case "security": return t("permissionGroupSecurity");
      case "audit": return t("permissionGroupAudit");
      case "jobs": return t("permissionGroupJobs");
      case "modules": return t("permissionGroupModules");
      case "updates": return t("permissionGroupUpdates");
      case "storage": return t("permissionGroupStorage");
      case "network": return t("permissionGroupNetwork");
      case "files": return t("permissionGroupFiles");
      default: return group;
    }
  }

  function templateFor(nextProfile: UserProfileName) {
    return catalog?.access.profiles.find((item) => item.name === nextProfile);
  }

  function resetForm() {
    setEditingUserID("");
    setUsername("");
    setDisplayName("");
    setPassword("");
    const defaultProfile: UserProfileName = "parent";
    setProfile(defaultProfile);
    setDisabled(false);
    setPermissions(templateFor(defaultProfile)?.default_permissions || []);
    setResourcePermissions([]);
    setFormError("");
  }

  function startCreate() {
    resetForm();
    setNotice("");
  }

  function startEdit(user: UserAccount) {
    setEditingUserID(user.id);
    setUsername(user.username);
    setDisplayName(user.display_name);
    setPassword("");
    setProfile(user.profile || "member");
    setDisabled(user.disabled);
    setPermissions([...(user.permissions || [])]);
    setResourcePermissions([...(user.resource_permissions || [])]);
    setFormError("");
    setNotice("");
  }

  function changeProfile(nextProfile: UserProfileName) {
    setProfile(nextProfile);
    const template = templateFor(nextProfile);
    if (template) {
      setPermissions([...template.default_permissions]);
    }
  }

  function togglePermission(permission: string, allowed: boolean) {
    setPermissions((current) => {
      const next = new Set(current);
      if (allowed) next.add(permission);
      else next.delete(permission);
      return [...next].sort();
    });
  }

  function hasResourcePermission(resourceID: string, permission: string) {
    return resourcePermissions.some((item) =>
      item.resource_type === "file_folder" &&
      item.resource_id === resourceID &&
      item.permission === permission
    );
  }

  function toggleResourcePermission(resourceID: string, permission: "files.read" | "files.write", allowed: boolean) {
    setResourcePermissions((current) => {
      const next = current.filter((item) =>
        !(item.resource_type === "file_folder" &&
          item.resource_id === resourceID &&
          item.permission === permission)
      );
      if (allowed) {
        next.push({permission, resource_type: "file_folder", resource_id: resourceID});
        if (permission === "files.write" && !next.some((item) =>
          item.resource_type === "file_folder" &&
          item.resource_id === resourceID &&
          item.permission === "files.read"
        )) {
          next.push({permission: "files.read", resource_type: "file_folder", resource_id: resourceID});
        }
      } else if (permission === "files.read") {
        return next.filter((item) =>
          !(item.resource_type === "file_folder" &&
            item.resource_id === resourceID &&
            item.permission === "files.write")
        );
      }
      return next;
    });
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setFormError("");
    setNotice("");
    try {
      let user: UserAccount;
      if (editingUser) {
        user = await api.updateUser(editingUser.id, {
          displayName,
          password: password || undefined,
          profile,
          disabled,
          permissions,
          resourcePermissions,
        });
        setNotice(t("userUpdated"));
      } else {
        user = await api.createUser({
          username,
          displayName,
          password,
          profile,
          permissions,
          resourcePermissions,
        });
        setNotice(t("userCreated"));
        window.dispatchEvent(new CustomEvent("home-ai-core:user-created", {detail: user.id}));
      }
      resetForm();
      resource.reload();
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setSaving(false);
    }
  }

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  const fullAccess = editingUser?.owner || templateFor(profile)?.full_access || false;
  const effectivePermissions = fullAccess
    ? permissionDefinitions.map((item) => item.name)
    : permissions;

  return (
    <div className="page">
      <PageHeading title={t("users")} subtitle={t("usersUnifiedSubtitle")} />

      <Panel title={t("userAccounts")} className="wide">
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("displayName")}</th>
                <th>{t("username")}</th>
                <th>{t("userProfile")}</th>
                <th>{t("userPermissions")}</th>
                <th>{t("status")}</th>
                <th>{t("lastLogin")}</th>
                {canManage && <th>{t("actions")}</th>}
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td>
                    <strong>{user.display_name}</strong>
                    {user.owner && <span className="status-badge">{t("userOwnerBadge")}</span>}
                  </td>
                  <td className="mono">{user.username}</td>
                  <td>{profileLabel(user.profile)}</td>
                  <td>{user.permissions?.length || 0}</td>
                  <td><Status value={user.disabled ? "disabled" : "active"} /></td>
                  <td>{user.last_login_at ? date(user.last_login_at) : "—"}</td>
                  {canManage && (
                    <td>
                      <button
                        type="button"
                        className="button secondary compact"
                        onClick={() => startEdit(user)}
                      >
                        {t("configure")}
                      </button>
                    </td>
                  )}
                </tr>
              ))}
              {users.length === 0 && (
                <tr><td colSpan={canManage ? 7 : 6} className="muted">{t("noUsers")}</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </Panel>

      {canManage && catalog && (
        <Panel title={editingUser ? t("editUser") : t("addUser")} className="wide">
          <form className="user-access-form" onSubmit={submit}>
            <div className="user-profile-grid">
              <label>
                {t("username")}
                <input
                  required
                  minLength={3}
                  maxLength={64}
                  autoComplete="off"
                  value={username}
                  disabled={!!editingUser}
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
                {editingUser ? t("newPasswordOptional") : t("password")}
                <input
                  required={!editingUser}
                  minLength={password ? 12 : undefined}
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </label>
              <label>
                {t("userProfile")}
                <select
                  value={profile}
                  disabled={!!editingUser?.owner}
                  onChange={(event) => changeProfile(event.target.value as UserProfileName)}
                >
                  {profileTemplates.map((item) => (
                    <option key={item.name} value={item.name}>
                      {profileLabel(item.name)}
                    </option>
                  ))}
                </select>
              </label>
              {editingUser && (
                <label className="user-disabled-control">
                  <input
                    type="checkbox"
                    checked={disabled}
                    disabled={editingUser.owner}
                    onChange={(event) => setDisabled(event.target.checked)}
                  />
                  <span>{t("userDisabled")}</span>
                </label>
              )}
            </div>

            <div className="user-access-section">
              <div className="user-access-section-head">
                <div>
                  <h3>{t("userCapabilities")}</h3>
                  <p className="muted small">{t("userCapabilitiesNotice")}</p>
                </div>
                {fullAccess && <span className="status-badge status-success">{t("userFullAccess")}</span>}
              </div>

              <div className="permission-groups">
                {groupedPermissions.map(([group, items]) => (
                  <fieldset className="permission-group" key={group}>
                    <legend>{permissionGroupLabel(group)}</legend>
                    {items.map((permission) => {
                      const checked = effectivePermissions.includes(permission.name);
                      return (
                        <label className="permission-option" key={permission.name}>
                          <input
                            type="checkbox"
                            checked={checked}
                            disabled={fullAccess}
                            onChange={(event) => togglePermission(permission.name, event.target.checked)}
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
            </div>

            <div className="user-access-section">
              <div className="user-access-section-head">
                <div>
                  <h3>{t("userResourceAccess")}</h3>
                  <p className="muted small">{t("userResourceAccessNotice")}</p>
                </div>
              </div>

              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{t("resource")}</th>
                      <th>{t("filePool")}</th>
                      <th>{t("fileReadOnly")}</th>
                      <th>{t("fileReadWrite")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {resources.map((item) => {
                      const globalRead = effectivePermissions.includes("files.read") || effectivePermissions.includes("files.manage");
                      const globalWrite = effectivePermissions.includes("files.write") || effectivePermissions.includes("files.manage");
                      const readScoped = hasResourcePermission(item.id, "files.read");
                      const writeScoped = hasResourcePermission(item.id, "files.write");
                      return (
                        <tr key={item.id}>
                          <td>
                            <strong>{item.name}</strong>
                            <div className="muted small">{item.kind || "file_folder"}</div>
                          </td>
                          <td>{item.container || "—"}</td>
                          <td>
                            <input
                              type="checkbox"
                              aria-label={`${item.name}: files.read`}
                              checked={globalRead || readScoped}
                              disabled={fullAccess || globalRead}
                              onChange={(event) => toggleResourcePermission(item.id, "files.read", event.target.checked)}
                            />
                          </td>
                          <td>
                            <input
                              type="checkbox"
                              aria-label={`${item.name}: files.write`}
                              checked={globalWrite || writeScoped}
                              disabled={fullAccess || globalWrite}
                              onChange={(event) => toggleResourcePermission(item.id, "files.write", event.target.checked)}
                            />
                          </td>
                        </tr>
                      );
                    })}
                    {resources.length === 0 && (
                      <tr><td colSpan={4} className="muted">{t("userNoResources")}</td></tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>

            {editingUser?.owner && <div className="notice">{t("userOwnerImmutableNotice")}</div>}
            {formError && <div className="form-error">{formError}</div>}
            {notice && <div className="storage-success">{notice}</div>}

            <div className="user-access-actions">
              {editingUser && (
                <button type="button" className="button secondary" disabled={saving} onClick={startCreate}>
                  {t("cancel")}
                </button>
              )}
              <button className="button primary" type="submit" disabled={saving}>
                {saving ? t("working") : editingUser ? t("saveChanges") : t("createUser")}
              </button>
            </div>
          </form>
        </Panel>
      )}
    </div>
  );
}
