import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { BlockNode } from "../api/types";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";

export function FilesPage({revision, canManage}: {revision: number; canManage: boolean}) {
  const {t, date} = useI18n();
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
  const [trashVisible, setTrashVisible] = useState(false);
  const [trashEntries, setTrashEntries] = useState<Awaited<ReturnType<typeof api.fileTrash>>>([]);
  const [trashLoading, setTrashLoading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [pendingUpload, setPendingUpload] = useState<{
    folderId: string;
    file: File;
    path: string;
    uploadId: string;
  } | null>(null);

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
    setTrashVisible(false);
    setTrashEntries([]);
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
    setUploadProgress(0);
    try {
      const path = joinFilePath(currentPath, file.name);
      const session = await api.startResumableUpload(selectedFolder.id, {
        path,
        sizeBytes: file.size,
      });
      const pending = {
        folderId: selectedFolder.id,
        file,
        path,
        uploadId: session.id,
      };
      setPendingUpload(pending);
      await continueUpload(pending);
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
      setBusy("");
    }
  }

  async function continueUpload(upload = pendingUpload) {
    if (!upload) return;
    setBusy("browser-upload");
    setBrowserError("");
    try {
      let status = await api.resumableUploadStatus(upload.folderId, upload.uploadId);
      let offset = status.received_bytes;
      setUploadProgress(status.size_bytes === 0 ? 100 : Math.round((offset / status.size_bytes) * 100));

      const chunkSize = 8 * 1024 * 1024;
      while (offset < upload.file.size) {
        const end = Math.min(offset + chunkSize, upload.file.size);
        const chunk = upload.file.slice(offset, end);
        const result = await api.uploadResumableChunk(upload.folderId, upload.uploadId, offset, chunk);
        status = result.upload;
        offset = status.received_bytes;
        setUploadProgress(status.size_bytes === 0 ? 100 : Math.round((offset / status.size_bytes) * 100));
      }

      await api.completeResumableUpload(upload.folderId, upload.uploadId);
      setPendingUpload(null);
      setUploadProgress(null);
      if (selectedFolder?.id === upload.folderId) {
        await loadEntries(upload.folderId, currentPath);
      }
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function cancelPendingUpload() {
    if (!pendingUpload) return;
    setBusy("browser-upload-cancel");
    setBrowserError("");
    try {
      await api.cancelResumableUpload(pendingUpload.folderId, pendingUpload.uploadId);
      setPendingUpload(null);
      setUploadProgress(null);
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function deleteEntry(path: string, name: string) {
    if (!selectedFolder?.can_write) return;
    if (!window.confirm(t("fileDeleteConfirm").replace("{name}", name))) return;
    setBusy("browser-delete");
    setBrowserError("");
    try {
      await api.deleteFileEntry(selectedFolder.id, path);
      await loadEntries(selectedFolder.id, currentPath);
      if (trashVisible) {
        await loadTrash(selectedFolder.id);
      }
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function moveEntry(path: string) {
    if (!selectedFolder?.can_write) return;
    const target = window.prompt(t("fileMovePrompt"), path);
    if (!target || target === path) return;
    setBusy("browser-move");
    setBrowserError("");
    try {
      await api.moveFileEntry(selectedFolder.id, path, target);
      await loadEntries(selectedFolder.id, currentPath);
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function loadTrash(folderID: string) {
    setTrashLoading(true);
    setBrowserError("");
    try {
      setTrashEntries(await api.fileTrash(folderID));
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setTrashLoading(false);
    }
  }

  async function toggleTrash() {
    if (!selectedFolder) return;
    const next = !trashVisible;
    setTrashVisible(next);
    if (next) {
      await loadTrash(selectedFolder.id);
    }
  }

  async function restoreTrashEntry(id: string) {
    if (!selectedFolder?.can_write) return;
    setBusy("trash-restore");
    setBrowserError("");
    try {
      await api.restoreFileTrash(selectedFolder.id, id);
      await Promise.all([
        loadTrash(selectedFolder.id),
        loadEntries(selectedFolder.id, currentPath),
      ]);
    } catch (reason) {
      setBrowserError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function purgeTrashEntry(id: string, name: string) {
    if (!selectedFolder?.can_write) return;
    if (!window.confirm(t("fileTrashPurgeConfirm").replace("{name}", name))) return;
    setBusy("trash-purge");
    setBrowserError("");
    try {
      await api.purgeFileTrash(selectedFolder.id, id);
      await loadTrash(selectedFolder.id);
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
            <button
              className={`button compact ${trashVisible ? "primary" : "secondary"}`}
              type="button"
              disabled={trashLoading}
              onClick={() => void toggleTrash()}
            >
              {t("fileTrash")}
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
                  disabled={busy !== "" || pendingUpload !== null}
                  onChange={(event) => {
                    const file = event.currentTarget.files?.[0];
                    event.currentTarget.value = "";
                    void uploadSelected(file);
                  }}
                />
              </label>
              {pendingUpload && pendingUpload.folderId === selectedFolder.id && (
                <div className="file-upload-resume">
                  <span className="muted">
                    {pendingUpload.file.name} · {uploadProgress ?? 0}%
                  </span>
                  <button
                    className="button compact secondary"
                    type="button"
                    disabled={busy !== ""}
                    onClick={() => void continueUpload()}
                  >
                    {t("fileUploadResume")}
                  </button>
                  <button
                    className="button compact danger"
                    type="button"
                    disabled={busy !== ""}
                    onClick={() => void cancelPendingUpload()}
                  >
                    {t("cancel")}
                  </button>
                </div>
              )}
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
                        <div className="network-actions">
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
                          {selectedFolder.can_write && entry.kind !== "symlink" && (
                            <>
                              <button
                                className="button compact secondary"
                                type="button"
                                disabled={busy !== ""}
                                onClick={() => void moveEntry(entry.path)}
                              >
                                {t("fileMoveRename")}
                              </button>
                              <button
                                className="button compact danger"
                                type="button"
                                disabled={busy !== ""}
                                onClick={() => void deleteEntry(entry.path, entry.name)}
                              >
                                {t("delete")}
                              </button>
                            </>
                          )}
                        </div>
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

          {trashVisible && (
            <div className="file-trash-section">
              <h3>{t("fileTrash")}</h3>
              {trashLoading ? (
                <div className="muted">{t("loading")}</div>
              ) : (
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>{t("name")}</th>
                        <th>{t("fileOriginalPath")}</th>
                        <th>{t("fileEntryType")}</th>
                        <th>{t("fileSize")}</th>
                        <th>{t("fileDeletedAt")}</th>
                        <th>{t("actions")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {trashEntries.map((entry) => (
                        <tr key={entry.id}>
                          <td>{entry.name}</td>
                          <td className="mono">{entry.original_path}</td>
                          <td>{entry.kind === "directory" ? t("fileDirectory") : t("fileRegularFile")}</td>
                          <td>{entry.kind === "file" ? formatFileSize(entry.size_bytes || 0) : "—"}</td>
                          <td>{date(entry.deleted_at)}</td>
                          <td>
                            {selectedFolder.can_write && (
                              <div className="network-actions">
                                <button
                                  className="button compact secondary"
                                  type="button"
                                  disabled={busy !== ""}
                                  onClick={() => void restoreTrashEntry(entry.id)}
                                >
                                  {t("fileRestore")}
                                </button>
                                <button
                                  className="button compact danger"
                                  type="button"
                                  disabled={busy !== ""}
                                  onClick={() => void purgeTrashEntry(entry.id, entry.name)}
                                >
                                  {t("fileDeletePermanently")}
                                </button>
                              </div>
                            )}
                          </td>
                        </tr>
                      ))}
                      {trashEntries.length === 0 && (
                        <tr>
                          <td colSpan={6} className="muted">{t("fileTrashEmpty")}</td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              )}
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

