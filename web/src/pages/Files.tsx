import { useCallback, useState, type FormEvent } from "react";
import { api } from "../api/client";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";

export function FilesPage({revision, canManage}: {revision: number; canManage: boolean}) {
  const {t} = useI18n();
  const load = useCallback(async () => {
    const folders = await api.fileFolders();
    if (!canManage) {
      return {folders, pools: [], users: []};
    }
    const [pools, users] = await Promise.all([api.filePools(), api.users()]);
    return {folders, pools, users};
  }, [canManage]);
  const resource = useResource(load, revision);

  const [poolName, setPoolName] = useState("");
  const [poolRoot, setPoolRoot] = useState("");
  const [folderPoolID, setFolderPoolID] = useState("");
  const [folderName, setFolderName] = useState("");
  const [folderKind, setFolderKind] = useState<"private" | "shared">("private");
  const [ownerUserID, setOwnerUserID] = useState("");
  const [busy, setBusy] = useState("");
  const [formError, setFormError] = useState("");
  const [notice, setNotice] = useState("");

  async function createPool(event: FormEvent) {
    event.preventDefault();
    setBusy("pool");
    setFormError("");
    setNotice("");
    try {
      await api.createFilePool({name: poolName, rootPath: poolRoot});
      setPoolName("");
      setPoolRoot("");
      setNotice(t("filePoolCreated"));
      resource.reload();
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function createFolder(event: FormEvent) {
    event.preventDefault();
    if (!folderPoolID) {
      setFormError(t("filePoolRequired"));
      return;
    }
    if (folderKind === "private" && !ownerUserID) {
      setFormError(t("fileOwnerRequired"));
      return;
    }
    setBusy("folder");
    setFormError("");
    setNotice("");
    try {
      await api.createFileFolder({
        poolId: folderPoolID,
        name: folderName,
        kind: folderKind,
        ownerUserId: folderKind === "private" ? ownerUserID : undefined,
      });
      setFolderName("");
      setNotice(t("fileFolderCreated"));
      resource.reload();
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  const folders = resource.data?.folders || [];
  const pools = resource.data?.pools || [];
  const users = resource.data?.users || [];
  const userName = new Map(users.map((user) => [user.id, user.display_name || user.username]));

  return (
    <div className="page">
      <PageHeading title={t("files")} subtitle={t("filesSubtitle")} />

      {formError && <div className="form-error">{formError}</div>}
      {notice && <div className="storage-success">{notice}</div>}

      <Panel title={t("fileFolders")} className="wide">
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("name")}</th>
                <th>{t("fileFolderType")}</th>
                <th>{t("filePool")}</th>
                <th>{t("fileOwner")}</th>
                <th>{t("fileAccess")}</th>
                {canManage && <th>{t("fileInternalPath")}</th>}
              </tr>
            </thead>
            <tbody>
              {folders.map((folder) => (
                <tr key={folder.id}>
                  <td><strong>{folder.name}</strong></td>
                  <td>{folder.kind === "private" ? t("filePrivate") : t("fileShared")}</td>
                  <td>{folder.pool_name}</td>
                  <td>
                    {folder.kind === "private"
                      ? (userName.get(folder.owner_user_id || "") || folder.owner_user_id || "—")
                      : t("fileAllMembers")}
                  </td>
                  <td>{folder.can_write ? t("fileReadWrite") : t("fileReadOnly")}</td>
                  {canManage && <td className="mono">{folder.relative_path}</td>}
                </tr>
              ))}
              {folders.length === 0 && (
                <tr>
                  <td colSpan={canManage ? 6 : 5} className="muted">{t("fileNoFolders")}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Panel>

      {canManage && (
        <div className="two-column">
          <Panel title={t("fileCreatePool")}>
            <form className="user-form" onSubmit={createPool}>
              <label>
                {t("name")}
                <input
                  required
                  maxLength={128}
                  value={poolName}
                  onChange={(event) => setPoolName(event.target.value)}
                  placeholder={t("filePoolNamePlaceholder")}
                />
              </label>
              <label>
                {t("filePoolRoot")}
                <input
                  required
                  value={poolRoot}
                  onChange={(event) => setPoolRoot(event.target.value)}
                  placeholder="/srv/home-ai/storage"
                />
              </label>
              <p className="muted small">{t("filePoolFoundationNotice")}</p>
              <button className="button primary" type="submit" disabled={busy !== ""}>
                {busy === "pool" ? t("working") : t("fileCreatePool")}
              </button>
            </form>
          </Panel>

          <Panel title={t("fileCreateFolder")}>
            <form className="user-form" onSubmit={createFolder}>
              <label>
                {t("filePool")}
                <select
                  required
                  value={folderPoolID}
                  onChange={(event) => setFolderPoolID(event.target.value)}
                >
                  <option value="">{t("fileChoosePool")}</option>
                  {pools.map((pool) => (
                    <option key={pool.id} value={pool.id}>
                      {pool.name} · {pool.root_path}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                {t("name")}
                <input
                  required
                  maxLength={128}
                  value={folderName}
                  onChange={(event) => setFolderName(event.target.value)}
                />
              </label>
              <label>
                {t("fileFolderType")}
                <select
                  value={folderKind}
                  onChange={(event) => setFolderKind(event.target.value as "private" | "shared")}
                >
                  <option value="private">{t("filePrivate")}</option>
                  <option value="shared">{t("fileShared")}</option>
                </select>
              </label>
              {folderKind === "private" && (
                <label>
                  {t("fileOwner")}
                  <select
                    required
                    value={ownerUserID}
                    onChange={(event) => setOwnerUserID(event.target.value)}
                  >
                    <option value="">{t("fileChooseUser")}</option>
                    {users.map((user) => (
                      <option key={user.id} value={user.id}>
                        {user.display_name || user.username}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <p className="muted small">
                {folderKind === "private" ? t("filePrivatePermissionNotice") : t("fileSharedPermissionNotice")}
              </p>
              <button
                className="button primary"
                type="submit"
                disabled={busy !== "" || pools.length === 0}
              >
                {busy === "folder" ? t("working") : t("fileCreateFolder")}
              </button>
            </form>
          </Panel>
        </div>
      )}
    </div>
  );
}
