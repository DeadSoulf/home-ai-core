import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { BlockNode } from "../api/types";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";

export function FilesPage({revision, canManage}: {revision: number; canManage: boolean}) {
  const {t} = useI18n();
  const load = useCallback(async () => {
    const folders = await api.fileFolders();
    if (!canManage) {
      return {folders, pools: [], users: [], system: undefined};
    }
    const [pools, users, system] = await Promise.all([api.filePools(), api.users(), api.system()]);
    return {folders, pools, users, system};
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
  const [selectedFolderID, setSelectedFolderID] = useState("");
  const [currentPath, setCurrentPath] = useState("");
  const [entries, setEntries] = useState<Awaited<ReturnType<typeof api.fileEntries>>>([]);
  const [entriesLoading, setEntriesLoading] = useState(false);
  const [browserError, setBrowserError] = useState("");
  const [newDirectoryName, setNewDirectoryName] = useState("");

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

  const selectedFolder = resource.data?.folders.find((folder) => folder.id === selectedFolderID);

  const loadEntries = useCallback(async (folderID: string, path: string) => {
    if (!folderID) {
      setEntries([]);
      return;
    }
    setEntriesLoading(true);
    setBrowserError("");
    try {
      setEntries(await api.fileEntries(folderID, path));
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setEntriesLoading(false);
    }
  }, [t]);

  useEffect(() => {
    if (selectedFolderID) {
      void loadEntries(selectedFolderID, currentPath);
    }
  }, [selectedFolderID, currentPath, loadEntries]);

  function openFolder(folderID: string) {
    setSelectedFolderID(folderID);
    setCurrentPath("");
    setEntries([]);
    setBrowserError("");
  }

  function openDirectory(path: string) {
    setCurrentPath(path);
  }

  function goUp() {
    if (!currentPath) return;
    const parts = currentPath.split("/").filter(Boolean);
    parts.pop();
    setCurrentPath(parts.join("/"));
  }

  async function createDirectory(event: FormEvent) {
    event.preventDefault();
    if (!selectedFolder || !selectedFolder.can_write || !newDirectoryName.trim()) return;
    setBusy("browser-directory");
    setBrowserError("");
    try {
      const path = joinFilePath(currentPath, newDirectoryName.trim());
      await api.createFileDirectory(selectedFolder.id, path);
      setNewDirectoryName("");
      await loadEntries(selectedFolder.id, currentPath);
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function uploadSelected(file?: File) {
    if (!file || !selectedFolder || !selectedFolder.can_write) return;
    setBusy("browser-upload");
    setBrowserError("");
    try {
      await api.uploadFile(selectedFolder.id, joinFilePath(currentPath, file.name), file);
      await loadEntries(selectedFolder.id, currentPath);
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function downloadEntry(path: string, name: string) {
    if (!selectedFolder) return;
    setBusy("browser-download");
    setBrowserError("");
    try {
      const blob = await api.downloadFile(selectedFolder.id, path);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = name;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  const folders = resource.data?.folders || [];
  const pools = resource.data?.pools || [];
  const users = resource.data?.users || [];
  const mountOptions = nasMountOptions(resource.data?.system?.system.block_tree || []);
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
                <th>{t("actions")}</th>
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
                  <td>
                    <button
                      className="button compact secondary"
                      type="button"
                      onClick={() => openFolder(folder.id)}
                    >
                      {t("fileOpen")}
                    </button>
                  </td>
                </tr>
              ))}
              {folders.length === 0 && (
                <tr>
                  <td colSpan={canManage ? 7 : 6} className="muted">{t("fileNoFolders")}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Panel>

      {selectedFolder && (
        <Panel title={selectedFolder.name} className="wide">
          <div className="file-browser-toolbar">
            <button
              className="button compact secondary"
              type="button"
              disabled={!currentPath || entriesLoading}
              onClick={goUp}
            >
              {t("fileUp")}
            </button>
            <span className="mono">{currentPath ? `/${currentPath}` : "/"}</span>
            <button
              className="button compact secondary"
              type="button"
              disabled={entriesLoading}
              onClick={() => void loadEntries(selectedFolder.id, currentPath)}
            >
              {t("refresh")}
            </button>
          </div>

          {selectedFolder.can_write && (
            <div className="file-browser-actions">
              <form onSubmit={createDirectory}>
                <input
                  value={newDirectoryName}
                  onChange={(event) => setNewDirectoryName(event.target.value)}
                  placeholder={t("fileNewDirectoryName")}
                  maxLength={255}
                  required
                />
                <button className="button primary compact" type="submit" disabled={busy !== ""}>
                  {t("fileCreateDirectory")}
                </button>
              </form>
              <label className="button secondary compact file-upload-button">
                {busy === "browser-upload" ? t("working") : t("fileUpload")}
                <input
                  type="file"
                  disabled={busy !== ""}
                  onChange={(event) => {
                    const file = event.currentTarget.files?.[0];
                    event.currentTarget.value = "";
                    void uploadSelected(file);
                  }}
                />
              </label>
            </div>
          )}

          {browserError && <div className="form-error">{browserError}</div>}
          {entriesLoading ? (
            <div className="muted">{t("loading")}</div>
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>{t("name")}</th>
                    <th>{t("fileEntryType")}</th>
                    <th>{t("fileSize")}</th>
                    <th>{t("modified")}</th>
                    <th>{t("actions")}</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((entry) => (
                    <tr key={entry.path}>
                      <td>
                        {entry.kind === "directory" ? (
                          <button
                            type="button"
                            className="file-entry-link"
                            onClick={() => openDirectory(entry.path)}
                          >
                            {entry.name}
                          </button>
                        ) : (
                          <span>{entry.name}</span>
                        )}
                      </td>
                      <td>{entry.kind === "directory" ? t("fileDirectory") : entry.kind === "file" ? t("fileRegularFile") : t("fileSymlink")}</td>
                      <td>{entry.kind === "file" ? formatFileSize(entry.size_bytes || 0) : "—"}</td>
                      <td>{new Date(entry.modified_at).toLocaleString()}</td>
                      <td>
                        {entry.kind === "file" && (
                          <button
                            className="button compact secondary"
                            type="button"
                            disabled={busy !== ""}
                            onClick={() => void downloadEntry(entry.path, entry.name)}
                          >
                            {t("fileDownload")}
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                  {entries.length === 0 && (
                    <tr>
                      <td colSpan={5} className="muted">{t("fileDirectoryEmpty")}</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          )}
        </Panel>
      )}

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
                <select
                  required
                  value={poolRoot}
                  onChange={(event) => setPoolRoot(event.target.value)}
                >
                  <option value="">{t("fileChooseMountedStorage")}</option>
                  {mountOptions.map((mount) => (
                    <option key={mount.path} value={mount.path}>
                      {mount.path} · {mount.filesystem || "filesystem"} · {mount.device || "—"}
                    </option>
                  ))}
                </select>
              </label>
              {mountOptions.length === 0 && <div className="notice">{t("fileNoMountedStorage")}</div>}
              <p className="muted small">{t("filePoolFoundationNotice")}</p>
              <button className="button primary" type="submit" disabled={busy !== "" || mountOptions.length === 0}>
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

type NASMountOption = {
  path: string;
  filesystem?: string;
  device?: string;
};

function nasMountOptions(nodes: BlockNode[]): NASMountOption[] {
  const byPath = new Map<string, NASMountOption>();
  const visit = (node: BlockNode) => {
    if (!node.system && node.filesystem) {
      for (const mountpoint of node.mountpoints || []) {
        if (mountpoint.startsWith("/mnt/home-ai-core/")) {
          byPath.set(mountpoint, {
            path: mountpoint,
            filesystem: node.filesystem,
            device: node.path || node.name,
          });
        }
      }
    }
    for (const child of node.children || []) visit(child);
  };
  for (const node of nodes) visit(node);
  return [...byPath.values()].sort((a, b) => a.path.localeCompare(b.path));
}

function joinFilePath(base: string, name: string): string {
  return [base, name].filter(Boolean).join("/");
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let value = bytes / 1024;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index++;
  }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[index]}`;
}

