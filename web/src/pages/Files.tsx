import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, APIError } from "../api/client";
import { ErrorNotice } from "../components/ErrorNotice";
import { describeFileError } from "../ui/fileErrors";
import type { StoragePurposeAssignment } from "../api/types";
import { FolderSettings } from "../components/FolderSettings";
import { SectionGuide } from "../components/SectionGuide";
import { filesGuides } from "../ui/experience";
import { useUI } from "../ui/useUI";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import type { FileSection } from "../navigation";
import { PageHeading } from "./Dashboard";

export function FilesPage({
  revision,
  canManage,
  canReadSystem,
  canManageStorage,
  section,
  onSectionChange,
}: {
  revision: number;
  canManage: boolean;
  canReadSystem: boolean;
  canManageStorage: boolean;
  section: FileSection;
  onSectionChange: (section: FileSection) => void;
}) {
  const {t, date, locale} = useI18n();
  const u = useUI();
  const load = useCallback(async () => {
    // A failure in optional administrator metadata must never hide the
    // user's accessible folders. Keep each failed section identifiable.
    const folders = await api.fileFolders();
    if (!canManage) {
      return {folders, pools: [], users: [], storagePurposes: [], smb: undefined,
        loadErrors: {pools: "", owners: "", storage: "", smb: ""}};
    }
    const [poolResult, ownerResult, storageResult, smbResult] = await Promise.allSettled([
      api.filePools(),
      api.fileOwners(),
      canReadSystem ? api.storagePurposes() : Promise.resolve([] as StoragePurposeAssignment[]),
      api.smbStatus(),
    ]);
    const message = (result: PromiseSettledResult<unknown>): string =>
      result.status === "rejected"
        ? (result.reason instanceof Error ? result.reason.message : u("requestFailed"))
        : "";
    return {
      folders,
      pools: poolResult.status === "fulfilled" ? poolResult.value : [],
      users: ownerResult.status === "fulfilled" ? ownerResult.value : [],
      storagePurposes: storageResult.status === "fulfilled" ? storageResult.value : [],
      smb: smbResult.status === "fulfilled" ? smbResult.value : undefined,
      loadErrors: {
        pools: message(poolResult),
        owners: message(ownerResult),
        storage: message(storageResult),
        smb: message(smbResult),
      },
    };
  }, [canManage, canReadSystem, u]);
  const resource = useResource(load, revision);

  const [settingsFolderID, setSettingsFolderID] = useState("");
  const [poolName, setPoolName] = useState("");
  const [poolRoot, setPoolRoot] = useState("");
  const [mountedStorageOverrides, setMountedStorageOverrides] = useState<Record<string, string>>({});
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
  const [formErrorCause, setFormErrorCause] = useState<unknown>();
  const [browserErrorCause, setBrowserErrorCause] = useState<unknown>();
  function reportFormError(message: string, cause?: unknown) {
    setFormError(message);
    setFormErrorCause(message ? cause : undefined);
  }
  function reportBrowserError(message: string, cause?: unknown) {
    setBrowserError(message);
    setBrowserErrorCause(message ? cause : undefined);
  }
  const [newDirectoryName, setNewDirectoryName] = useState("");
  const [trashVisible, setTrashVisible] = useState(false);
  const [trashEntries, setTrashEntries] = useState<Awaited<ReturnType<typeof api.fileTrash>>>([]);
  const [trashLoading, setTrashLoading] = useState(false);
  const [uploadsVisible, setUploadsVisible] = useState(false);
  const [pendingUploads, setPendingUploads] = useState<Awaited<ReturnType<typeof api.fileUploads>>>([]);
  const [pendingUploadsLoading, setPendingUploadsLoading] = useState(false);
  const [deleteBlockers, setDeleteBlockers] = useState<{folderID: string; counts: FolderDeletionBlockers}>();
  const [uploadProgress, setUploadProgress] = useState<{
    name: string;
    received: number;
    total: number;
    resumed: boolean;
  }>();
  const [smbWorkgroup, setSMBWorkgroup] = useState("WORKGROUP");
  const [smbUserID, setSMBUserID] = useState("");
  const [smbPassword, setSMBPasswordText] = useState("");

  useEffect(() => {
    const smb = resource.data?.smb;
    if (!smb) return;
    setSMBWorkgroup((current) => current === "WORKGROUP" ? (smb.workgroup || "WORKGROUP") : current);
    setSMBUserID((current) => current || smb.users[0]?.user_id || "");
  }, [resource.data?.smb]);

  async function installSMB() {
    setBusy("smb-install");
    reportFormError("");
    setNotice("");
    try {
      const result = await api.smbOperation({operation: "install"});
      setNotice(result.message);
      resource.reload();
    } catch (reason) {
      reportFormError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function applySMB() {
    setBusy("smb-apply");
    reportFormError("");
    setNotice("");
    try {
      const result = await api.smbOperation({
        operation: "apply",
        workgroup: smbWorkgroup,
      });
      setNotice(result.message);
      resource.reload();
    } catch (reason) {
      reportFormError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function submitSMBPassword(event: FormEvent) {
    event.preventDefault();
    if (!smbUserID || smbPassword.length < 12) {
      reportFormError(t("smbPasswordRequirement"));
      return;
    }
    setBusy("smb-password");
    reportFormError("");
    setNotice("");
    try {
      const result = await api.smbOperation({
        operation: "set_password",
        userId: smbUserID,
        password: smbPassword,
      });
      setSMBPasswordText("");
      setNotice(result.message);
      resource.reload();
    } catch (reason) {
      reportFormError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function mountFileStorage(storage: StoragePurposeAssignment) {
    if (!canManageStorage) return;
    const busyKey = `storage-mount-${storage.device}`;
    setBusy(busyKey);
    reportFormError("");
    setNotice("");
    try {
      const result = await api.storageOperation({operation: "mount", device: storage.device});
      const deviceName = storage.device.split("/").filter(Boolean).pop() || "";
      const mountpoint = result.mountpoint || (deviceName ? `/mnt/home-ai-core/${deviceName}` : "");
      if (mountpoint) {
        setMountedStorageOverrides((current) => ({...current, [storage.device]: mountpoint}));
        setPoolRoot(mountpoint);
      }
      setNotice(t("fileStorageMounted"));
      resource.reload();
    } catch (reason) {
      reportFormError(localizeFileStorageError(reason, locale, t("requestFailed"), t("fileStorageMountFailed"), t("fileStorageMountedElsewhere"), t("fileStorageFilesystemError")), reason);
    } finally {
      setBusy("");
    }
  }

  async function createPool(event: FormEvent) {
    event.preventDefault();
    setBusy("pool");
    reportFormError("");
    setNotice("");
    try {
      await api.createFilePool({name: poolName, rootPath: poolRoot});
      setPoolName("");
      setPoolRoot("");
      setNotice(t("filePoolCreated"));
      resource.reload();
    } catch (reason) {
      const message = reason instanceof APIError ? reason.rawMessage : reason instanceof Error ? reason.message : "";
      if (locale === "ru" && message.includes("file pool root must be a mounted storage device explicitly assigned to Files")) {
        reportFormError(t("filePoolStorageNotReady"), reason);
      } else {
        reportFormError(describeFileError(reason, locale, t("requestFailed")), reason);
      }
    } finally {
      setBusy("");
    }
  }

  async function updatePoolCapacityPolicy(event: FormEvent<HTMLFormElement>, poolId: string) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const reservePercent = Number(data.get("reserve_percent"));
    const warningPercent = Number(data.get("warning_percent"));
    setBusy(`pool-policy-${poolId}`);
    reportFormError("");
    setNotice("");
    try {
      await api.updateFilePoolCapacityPolicy(poolId, {reservePercent, warningPercent});
      setNotice(t("filePoolPolicySaved"));
      resource.reload();
    } catch (reason) {
      reportFormError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function createFolder(event: FormEvent) {
    event.preventDefault();
    if (!folderPoolID) {
      reportFormError(t("filePoolRequired"));
      return;
    }
    if (folderKind === "private" && !ownerUserID) {
      reportFormError(t("fileOwnerRequired"));
      return;
    }
    setBusy("folder");
    reportFormError("");
    setNotice("");
    try {
      const created = await api.createFileFolder({
        poolId: folderPoolID,
        name: folderName,
        kind: folderKind,
        ownerUserId: folderKind === "private" ? ownerUserID : undefined,
      });
      setFolderName("");
      setNotice(created.warning || t("fileFolderCreated"));
      resource.reload();
    } catch (reason) {
      reportFormError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function deleteLogicalFolder(folder: {id: string; name: string}) {
    if (!canManage || busy !== "") return;
    const confirmation = window.prompt(
      u("fileDeleteFolderPrompt").replace("{name}", folder.name),
    );
    if (confirmation === null) return;
    if (confirmation !== folder.name) {
      reportFormError(u("fileDeleteFolderMismatch"));
      return;
    }
    setBusy("folder-delete-" + folder.id);
    reportFormError("");
    setDeleteBlockers(undefined);
    setNotice("");
    try {
      const result = await api.deleteFileFolder(folder.id, confirmation);
      if (selectedFolderID === folder.id) {
        setSelectedFolderID("");
        setCurrentPath("");
        setEntries([]);
        setTrashVisible(false);
        setTrashEntries([]);
        reportBrowserError("");
      }
      if (settingsFolderID === folder.id) setSettingsFolderID("");
      setDeleteBlockers(undefined);
      setNotice(result.warning || u("fileDeleteFolderDone"));
      resource.reload();
    } catch (reason) {
      if (reason instanceof APIError && reason.code === "file_folder_not_empty") {
        const counts = parseFolderDeletionBlockers(reason.details?.blockers);
        if (counts) setDeleteBlockers({folderID: folder.id, counts});
      }
      reportFormError(describeFileError(reason, locale, t("requestFailed")), reason);
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
    reportBrowserError("");
    try {
      setEntries(await api.fileEntries(folderID, path));
    } catch (reason) {
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setEntriesLoading(false);
    }
  }, [t, locale]);

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
    setUploadsVisible(false);
    setPendingUploads([]);
    reportBrowserError("");
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
    reportBrowserError("");
    try {
      const path = joinFilePath(currentPath, newDirectoryName.trim());
      await api.createFileDirectory(selectedFolder.id, path);
      setNewDirectoryName("");
      await loadEntries(selectedFolder.id, currentPath);
    } catch (reason) {
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function uploadSelected(file?: File) {
    if (!file || !selectedFolder || !selectedFolder.can_write) return;
    setBusy("browser-upload");
    reportBrowserError("");
    setNotice("");

    const targetPath = joinFilePath(currentPath, file.name);
    const fingerprint = `${file.name}:${file.size}:${file.lastModified}`;
    try {
      const uploads = await api.fileUploads(selectedFolder.id);
      let session = uploads.find((item) =>
        item.path === targetPath &&
        item.total_bytes === file.size &&
        item.client_fingerprint === fingerprint
      );

      if (!session) {
        const stale = uploads.find((item) => item.path === targetPath);
        if (stale) {
          if (!window.confirm(t("fileUploadRestartConfirm").replace("{name}", file.name))) {
            return;
          }
          await api.cancelFileUpload(selectedFolder.id, stale.id);
        }
        session = await api.createFileUpload(selectedFolder.id, {
          path: targetPath,
          totalBytes: file.size,
          clientFingerprint: fingerprint,
        });
      }

      let resumed = session.received_bytes > 0;
      if (resumed && session.chunks?.length) {
        const verified = await verifyUploadedChunks(file, session.chunks);
        if (!verified) {
          await api.cancelFileUpload(selectedFolder.id, session.id);
          session = await api.createFileUpload(selectedFolder.id, {
            path: targetPath,
            totalBytes: file.size,
            clientFingerprint: fingerprint,
          });
          resumed = false;
        }
      }

      setUploadProgress({
        name: file.name,
        received: session.received_bytes,
        total: file.size,
        resumed,
      });

      while (session.received_bytes < file.size) {
        const offset = session.received_bytes;
        const chunk = file.slice(offset, Math.min(offset + fileUploadChunkBytes, file.size));
        const checksum = await sha256Blob(chunk);
        session = await api.uploadFileChunk(
          selectedFolder.id,
          session.id,
          offset,
          chunk,
          checksum,
        );
        setUploadProgress({
          name: file.name,
          received: session.received_bytes,
          total: file.size,
          resumed,
        });
      }

      const result = await api.completeFileUpload(selectedFolder.id, session.id);
      setNotice(t("fileUploadCompleteChecksum").replace("{sha256}", result.sha256));
      await loadEntries(selectedFolder.id, currentPath);
    } catch (reason) {
      const message = describeFileError(reason, locale, t("requestFailed"));
      reportBrowserError(`${t("fileUploadInterrupted")} ${message}`, reason);
    } finally {
      setUploadProgress(undefined);
      setBusy("");
    }
  }

  async function deleteEntry(path: string, name: string) {
    if (!selectedFolder?.can_write) return;
    if (!window.confirm(t("fileDeleteConfirm").replace("{name}", name))) return;
    setBusy("browser-delete");
    reportBrowserError("");
    try {
      await api.deleteFileEntry(selectedFolder.id, path);
      await loadEntries(selectedFolder.id, currentPath);
      if (trashVisible) {
        await loadTrash(selectedFolder.id);
      }
    } catch (reason) {
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function moveEntry(path: string) {
    if (!selectedFolder?.can_write) return;
    const target = window.prompt(t("fileMovePrompt"), path);
    if (!target || target === path) return;
    setBusy("browser-move");
    reportBrowserError("");
    try {
      await api.moveFileEntry(selectedFolder.id, path, target);
      await loadEntries(selectedFolder.id, currentPath);
    } catch (reason) {
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function loadTrash(folderID: string) {
    setTrashLoading(true);
    reportBrowserError("");
    try {
      setTrashEntries(await api.fileTrash(folderID));
    } catch (reason) {
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
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

  async function loadPendingUploads(folderID: string) {
    setPendingUploadsLoading(true);
    reportBrowserError("");
    try {
      setPendingUploads(await api.fileUploads(folderID));
    } catch (reason) {
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setPendingUploadsLoading(false);
    }
  }

  async function togglePendingUploads() {
    if (!selectedFolder?.can_write) return;
    const next = !uploadsVisible;
    setUploadsVisible(next);
    if (next) await loadPendingUploads(selectedFolder.id);
  }

  async function cancelPendingUpload(upload: {id: string; path: string}) {
    if (!selectedFolder?.can_write || busy !== "") return;
    if (!window.confirm(u("fileCancelUploadConfirm").replace("{name}", upload.path))) return;
    setBusy("upload-cancel");
    reportBrowserError("");
    try {
      await api.cancelFileUpload(selectedFolder.id, upload.id);
      setDeleteBlockers(undefined);
      reportFormError("");
      setNotice(u("fileCancelUploadComplete"));
      await loadPendingUploads(selectedFolder.id);
    } catch (reason) {
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function inspectBlockingTrash(folderID: string) {
    openFolder(folderID);
    setTrashVisible(true);
    await loadTrash(folderID);
  }

  async function inspectBlockingUploads(folderID: string) {
    openFolder(folderID);
    setUploadsVisible(true);
    await loadPendingUploads(folderID);
  }

  async function restoreTrashEntry(id: string) {
    if (!selectedFolder?.can_write) return;
    setBusy("trash-restore");
    reportBrowserError("");
    try {
      await api.restoreFileTrash(selectedFolder.id, id);
      await Promise.all([
        loadTrash(selectedFolder.id),
        loadEntries(selectedFolder.id, currentPath),
      ]);
    } catch (reason) {
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function purgeTrashEntry(id: string, name: string) {
    if (!selectedFolder?.can_write) return;
    if (!window.confirm(t("fileTrashPurgeConfirm").replace("{name}", name))) return;
    setBusy("trash-purge");
    reportBrowserError("");
    try {
      await api.purgeFileTrash(selectedFolder.id, id);
      await loadTrash(selectedFolder.id);
    } catch (reason) {
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  async function downloadEntry(path: string, name: string) {
    if (!selectedFolder) return;
    setBusy("browser-download");
    reportBrowserError("");
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
      reportBrowserError(describeFileError(reason, locale, t("requestFailed")), reason);
    } finally {
      setBusy("");
    }
  }

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <div className="page"><ErrorNotice error={resource.failure} message={describeFileError(resource.failure, locale, resource.error)} /><button className="button secondary" type="button" onClick={resource.reload}>{u("retry")}</button></div>;

  const folders = resource.data?.folders || [];
  const pools = resource.data?.pools || [];
  const users = resource.data?.users || [];
  const fileStorage = (resource.data?.storagePurposes || []).filter((item) => item.purpose === "files");
  const effectiveFileStorage = fileStorage.map((storage) => {
    const override = mountedStorageOverrides[storage.device];
    const mountpoints = storage.mountpoints || [];
    if (!override || mountpoints.includes(override)) return storage;
    return {...storage, mountpoints: [...mountpoints, override]};
  });
  const mountOptions = fileStorageMountOptions(effectiveFileStorage);
  const poolRoots = new Set(pools.map((pool) => pool.root_path));
  const smbStatus = resource.data?.smb;
  const userName = new Map(users.map((user) => [user.id, user.display_name || user.username]));
  const loadErrors = resource.data?.loadErrors;
  const canShowStorageInventory = canReadSystem && !loadErrors?.storage;

  return (
    <div className="page">
      <PageHeading title={t("files")} subtitle={t("filesSubtitle")} />

      {canManage && (section === "folders" || section === "storage") && loadErrors?.pools &&
        <ErrorState message={u("filesPoolsLoadFailed") + " " + loadErrors.pools} onRetry={resource.reload} />}
      {canManage && section === "folders" && loadErrors?.owners &&
        <ErrorState message={u("filesOwnersLoadFailed") + " " + loadErrors.owners} onRetry={resource.reload} />}
      {canManage && section === "storage" && !canReadSystem &&
        <ErrorState message={u("filesSystemReadRequired")} />}
      {canManage && section === "storage" && canReadSystem && loadErrors?.storage &&
        <ErrorState message={u("filesStorageLoadFailed") + " " + loadErrors.storage} onRetry={resource.reload} />}
      {canManage && section === "storage" && !canManageStorage &&
        <p className="notice" role="status">{u("filesStorageManageRequired")}</p>}
      {canManage && section === "windows" && loadErrors?.smb &&
        <ErrorState message={u("filesSMBLoadFailed") + " " + loadErrors.smb} onRetry={resource.reload} />}

      {formError && (formErrorCause ? <ErrorNotice error={formErrorCause} message={formError} /> : <div className="form-error" role="alert">{formError}</div>)}
      {notice && <div className="storage-success">{notice}</div>}

      {canManage && <nav className="file-section-tabs" aria-label={t("files")}>
        {(["folders", "storage", "windows"] as const).map((item) => <button key={item} type="button"
          className={`button ${section === item ? "primary" : "secondary"}`} aria-current={section === item ? "page" : undefined}
          onClick={() => onSectionChange(item)}>{t(item === "folders" ? "fileFolders" : item === "storage" ? "fileStorageSettings" : "fileWindowsSettings")}</button>)}
      </nav>}
      <SectionGuide title={u(filesGuides[section].title)} description={u(filesGuides[section].description)} />
      {settingsFolderID && section === "folders" && <FolderSettings folderID={settingsFolderID} users={users}
        onClose={() => setSettingsFolderID("")} onSaved={() => resource.reload()}/>}

      {section === "folders" && <Panel title={t("fileFolders")} className="wide">
        <div className="ux-folder-grid">
          {folders.map((folder) => (
            <article className="ux-folder-card" key={folder.id}>
              <div className="ux-folder-head">
                <span className="ux-folder-symbol" aria-hidden="true">▣</span>
                <div className="ux-folder-name">
                  <h3>{folder.name}</h3>
                  <span>{folder.kind === "private" ? t("filePrivate") : t("fileShared")} · {folder.pool_name}</span>
                </div>
                <span className="ux-folder-access">
                  {folder.can_write ? t("fileReadWrite") : t("fileReadOnly")}
                </span>
              </div>
              <dl className="ux-folder-facts">
                <div>
                  <dt>{t("fileOwner")}</dt>
                  <dd>{folder.kind === "private"
                    ? (userName.get(folder.owner_user_id || "") || folder.owner_user_id || "—")
                    : t("fileAllMembers")}</dd>
                </div>
                <div>
                  <dt>{t("fileFolderUsage")}</dt>
                  <dd>{folder.usage_known
                    ? formatFileSize(folder.used_bytes + folder.reserved_bytes) + " / " + (folder.quota_bytes ? formatFileSize(folder.quota_bytes) : t("quotaUnlimited"))
                    : t("usageUnavailable")}</dd>
                </div>
              </dl>
              <div className="ux-folder-actions">
                <button className="button primary" type="button" onClick={() => openFolder(folder.id)}>
                  {t("fileOpen")}
                </button>
                {canManage && <button className="button secondary" type="button"
                  onClick={() => setSettingsFolderID(folder.id)}>{t("fileFolderSettings")}</button>}
                {canManage && <button className="button danger" type="button"
                  disabled={busy !== ""}
                  onClick={() => void deleteLogicalFolder(folder)}>{u("fileDeleteFolder")}</button>}
              </div>
              {deleteBlockers?.folderID === folder.id && (
                <div className="notice folder-delete-blockers" role="status">
                  <strong>{u("fileDeleteHiddenData")}</strong>
                  <ul>
                    {deleteBlockers.counts.visible_entries > 0 && <li>{u("fileDeleteVisibleCount").replace("{count}", String(deleteBlockers.counts.visible_entries))}</li>}
                    {deleteBlockers.counts.upload_artifacts > 0 && <li>{u("fileDeleteUploadCount").replace("{count}", String(deleteBlockers.counts.upload_artifacts))}</li>}
                    {deleteBlockers.counts.trash_items > 0 && <li>{u("fileDeleteTrashItemsCount").replace("{count}", String(deleteBlockers.counts.trash_items))}</li>}
                    {deleteBlockers.counts.trash_metadata > 0 && <li>{u("fileDeleteTrashMetadataCount").replace("{count}", String(deleteBlockers.counts.trash_metadata))}</li>}
                    {deleteBlockers.counts.other_hidden > 0 && <li>{u("fileDeleteOtherCount").replace("{count}", String(deleteBlockers.counts.other_hidden))}</li>}
                  </ul>
                  <div className="network-actions">
                    {(deleteBlockers.counts.trash_items > 0 || deleteBlockers.counts.trash_metadata > 0) && (
                      <button className="button compact secondary" type="button"
                        onClick={() => void inspectBlockingTrash(folder.id)}>{u("fileDeleteInspectTrash")}</button>
                    )}
                    {deleteBlockers.counts.upload_artifacts > 0 && (
                      <button className="button compact secondary" type="button"
                        onClick={() => void inspectBlockingUploads(folder.id)}>{u("fileDeleteInspectUploads")}</button>
                    )}
                  </div>
                  {(deleteBlockers.counts.other_hidden > 0 || deleteBlockers.counts.upload_artifacts > 0 || deleteBlockers.counts.trash_items > 0 || deleteBlockers.counts.trash_metadata > 0) &&
                    <p className="muted small">{u("fileDeleteManualInspection")}</p>}
                </div>
              )}
            </article>
          ))}
          {folders.length === 0 && <p className="muted">{t("fileNoFolders")}</p>}
        </div>
      </Panel>}

      {section === "folders" && selectedFolder && (
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
            {selectedFolder.can_write && (
              <button className={"button compact " + (uploadsVisible ? "primary" : "secondary")}
                type="button" disabled={pendingUploadsLoading}
                onClick={() => void togglePendingUploads()}>{u("filePendingUploads")}</button>
            )}
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

          {uploadProgress && (
            <div className="file-upload-progress">
              <progress
                max={Math.max(uploadProgress.total, 1)}
                value={uploadProgress.received}
              />
              <span>
                {t("fileUploadProgress")
                  .replace("{name}", uploadProgress.name)
                  .replace(
                    "{percent}",
                    String(uploadProgress.total === 0
                      ? 100
                      : Math.floor((uploadProgress.received / uploadProgress.total) * 100)),
                  )}
                {uploadProgress.resumed ? ` · ${t("fileUploadResumed")}` : ""}
              </span>
            </div>
          )}

          {browserError && (browserErrorCause ? <ErrorNotice error={browserErrorCause} message={browserError} /> : <div className="form-error" role="alert">{browserError}</div>)}
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

          {uploadsVisible && selectedFolder.can_write && (
            <div className="file-pending-uploads">
              <h3>{u("filePendingUploads")}</h3>
              {pendingUploadsLoading ? <p className="muted">{t("loading")}</p> : pendingUploads.length > 0 ? (
                <div className="table-wrap">
                  <table>
                    <thead><tr><th>{t("name")}</th><th>{t("progress")}</th><th>{t("actions")}</th></tr></thead>
                    <tbody>
                      {pendingUploads.map((upload) => (
                        <tr key={upload.id}>
                          <td className="mono">{upload.path}</td>
                          <td>{formatFileSize(upload.received_bytes)} / {formatFileSize(upload.total_bytes)}</td>
                          <td><button className="button compact danger" type="button"
                            disabled={busy !== ""} onClick={() => void cancelPendingUpload(upload)}>
                            {u("fileCancelUpload")}
                          </button></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : <p className="notice">{u("filePendingUploadsEmpty")}</p>}
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

      {canManage && section === "windows" && smbStatus && (
        <Panel title={t("smbTitle")} className="wide">
          <div className="smb-status-grid">
            <dl className="details">
              <dt>{t("state")}</dt>
              <dd>{smbStatus.available ? (smbStatus.active ? t("smbActive") : t("smbInactive")) : t("smbNotInstalled")}</dd>
              <dt>{t("hostname")}</dt>
              <dd className="mono">{smbStatus.hostname || "—"}</dd>
              <dt>{t("smbWorkgroup")}</dt>
              <dd>{smbStatus.workgroup || "WORKGROUP"}</dd>
              <dt>{t("smbHardQuota")}</dt>
              <dd>
                <span className={smbStatus.hard_quota_ready ? "status-badge status-success" : "status-badge status-failed"}>
                  {smbStatus.hard_quota_ready ? t("smbHardQuotaReady") : t("smbHardQuotaNotReady")}
                </span>
              </dd>
            </dl>
            {!smbStatus.available ? (
              <div className="smb-install">
                <div className="notice">{smbStatus.error || t("smbNotInstalledNotice")}</div>
                <button
                  type="button"
                  className="button primary"
                  disabled={busy !== ""}
                  onClick={() => void installSMB()}
                >
                  {busy === "smb-install" ? t("working") : t("smbInstall")}
                </button>
              </div>
            ) : (
              <div className="smb-actions">
                <label>
                  {t("smbWorkgroup")}
                  <input
                    value={smbWorkgroup}
                    onChange={(event) => setSMBWorkgroup(event.target.value.toUpperCase())}
                    maxLength={15}
                  />
                </label>
                <button
                  type="button"
                  className="button primary"
                  disabled={busy !== ""}
                  onClick={() => void applySMB()}
                >
                  {busy === "smb-apply" ? t("working") : t("smbApply")}
                </button>
              </div>
            )}
          </div>

          {smbStatus.available && !smbStatus.hard_quota_ready && (
            <div className="notice">
              <strong>{t("smbHardQuotaNotReady")}</strong>
              <div>{smbStatus.hard_quota_error || t("smbHardQuotaMigrationNotice")}</div>
              <button
                type="button"
                className="button compact secondary"
                disabled={busy !== ""}
                onClick={() => void installSMB()}
              >
                {busy === "smb-install" ? t("working") : t("smbInstallQuotaSupport")}
              </button>
            </div>
          )}

          {smbStatus.available && (
            <>
              <h3>{t("smbShares")}</h3>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{t("fileFolders")}</th>
                      <th>{t("smbShareName")}</th>
                      <th>{t("smbWindowsPath")}</th>
                      <th>{t("fileAccess")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {smbStatus.shares.map((share) => (
                      <tr key={share.folder_id}>
                        <td>{share.folder_name}</td>
                        <td className="mono">{share.share_name}</td>
                        <td className="mono">{share.unc}</td>
                        <td>{share.writable ? t("fileReadWrite") : t("fileReadOnly")}</td>
                      </tr>
                    ))}
                    {smbStatus.shares.length === 0 && (
                      <tr><td colSpan={4} className="muted">{t("smbNoShares")}</td></tr>
                    )}
                  </tbody>
                </table>
              </div>

              <p className="muted small">{t("filePoolSMBReserveNotice")}</p>
              <h3>{t("smbUsers")}</h3>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{t("user")}</th>
                      <th>{t("smbUsername")}</th>
                      <th>{t("state")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {smbStatus.users.map((user) => (
                      <tr key={user.user_id}>
                        <td>{user.display_name || user.username}</td>
                        <td className="mono">{user.smb_username}</td>
                        <td>{user.configured ? t("smbPasswordConfigured") : t("smbPasswordMissing")}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <form className="smb-password-form" onSubmit={submitSMBPassword}>
                <label>
                  {t("user")}
                  <select value={smbUserID} onChange={(event) => setSMBUserID(event.target.value)} required>
                    <option value="">{t("fileChooseUser")}</option>
                    {smbStatus.users.map((user) => (
                      <option key={user.user_id} value={user.user_id}>
                        {user.display_name || user.username} · {user.smb_username}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {t("smbPassword")}
                  <input
                    type="password"
                    minLength={12}
                    maxLength={256}
                    value={smbPassword}
                    onChange={(event) => setSMBPasswordText(event.target.value)}
                    autoComplete="new-password"
                    required
                  />
                </label>
                <button className="button secondary" type="submit" disabled={busy !== ""}>
                  {busy === "smb-password" ? t("working") : t("smbSetPassword")}
                </button>
                <p className="muted small">{t("smbPasswordNotice")}</p>
              </form>
            </>
          )}
        </Panel>
      )}

      {canManage && section === "windows" && !smbStatus && !loadErrors?.smb && <div className="notice">{t("fileSMBUnavailable")}</div>}
      {canManage && section === "storage" && (
        <Panel title={t("filePoolsTitle")} className="wide">
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("name")}</th>
                  <th>{t("filePoolCapacity")}</th>
                  <th>{t("state")}</th>
                  <th>{t("filePoolCapacityPolicy")}</th>
                </tr>
              </thead>
              <tbody>
                {pools.map((pool) => {
                  const stateLabel =
                    pool.capacity_state === "reserve"
                      ? t("filePoolCapacityReserve")
                      : pool.capacity_state === "warning"
                        ? t("filePoolCapacityWarning")
                        : pool.capacity_state === "ok"
                          ? t("filePoolCapacityOK")
                          : t("filePoolCapacityUnknown");
                  const stateClass =
                    pool.capacity_state === "reserve"
                      ? "status-badge status-failed"
                      : pool.capacity_state === "ok"
                        ? "status-badge status-success"
                        : "status-badge";
                  return (
                    <tr key={pool.id}>
                      <td>
                        <strong>{pool.name}</strong>
                        <div className="muted small mono">{pool.root_path}</div>
                      </td>
                      <td>
                        {pool.capacity_known
                          ? `${formatFileSize(pool.free_bytes || 0)} / ${formatFileSize(pool.size_bytes || 0)}`
                          : t("filePoolCapacityUnknown")}
                      </td>
                      <td>
                        <span className={stateClass}>{stateLabel}</span>
                      </td>
                      <td>
                        <form
                          key={`${pool.id}-${pool.reserve_percent}-${pool.warning_percent}`}
                          className="network-actions"
                          onSubmit={(event) => void updatePoolCapacityPolicy(event, pool.id)}
                        >
                          <label>
                            {t("filePoolReservePercent")}
                            <input
                              name="reserve_percent"
                              type="number"
                              min={0}
                              max={50}
                              step={1}
                              required
                              defaultValue={pool.reserve_percent}
                            />
                          </label>
                          <label>
                            {t("filePoolWarningPercent")}
                            <input
                              name="warning_percent"
                              type="number"
                              min={0}
                              max={95}
                              step={1}
                              required
                              defaultValue={pool.warning_percent}
                            />
                          </label>
                          <button
                            className="button compact secondary"
                            type="submit"
                            disabled={busy !== ""}
                          >
                            {busy === `pool-policy-${pool.id}` ? t("working") : t("filePoolPolicySave")}
                          </button>
                        </form>
                        {pool.capacity_known && (
                          <div className="muted small">
                            {t("filePoolReserveDetails")
                              .replace("{reserve}", formatFileSize(pool.reserve_bytes || 0))
                              .replace("{warning}", formatFileSize(pool.warning_bytes || 0))}
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
                {pools.length === 0 && (
                  <tr>
                    <td colSpan={4} className="muted">{t("fileNoPools")}</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <p className="muted small">{t("filePoolCapacityNotice")}</p>
          <p className="muted small">{t("filePoolSMBReserveNotice")}</p>
        </Panel>
      )}

      {canManage && section === "storage" && canShowStorageInventory && (
        <Panel title={t("fileAssignedStorage")} className="wide">
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("device")}</th>
                  <th>{t("name")}</th>
                  <th>{t("filesystem")}</th>
                  <th>{t("mountPoints")}</th>
                  <th>{t("freeSpace")}</th>
                  <th>{t("state")}</th>
                  <th>{t("actions")}</th>
                </tr>
              </thead>
              <tbody>
                {effectiveFileStorage.map((storage) => {
                  const mountpoints = storage.mountpoints || [];
                  const usableMounts = mountpoints.filter((path) => path.startsWith("/mnt/home-ai-core/"));
                  const alreadyPool = storage.in_use || usableMounts.some((path) => poolRoots.has(path));
                  return (
                    <tr key={storage.filesystem_uuid || storage.device}>
                      <td className="mono">{storage.device}</td>
                      <td>{storage.label || "—"}</td>
                      <td>{storage.filesystem || "—"}</td>
                      <td className="mono">{mountpoints.join(", ") || "—"}</td>
                      <td>{storage.free_known ? formatFileSize(storage.free_bytes || 0) : t("filePoolCapacityUnknown")}</td>
                      <td>
                        {!storage.present
                          ? t("fileStorageMissing")
                          : !storage.filesystem
                            ? t("fileStorageNeedsFormat")
                            : alreadyPool
                              ? t("fileStoragePoolReady")
                              : usableMounts.length > 0
                                ? t("fileStorageReady")
                                : t("fileStorageNeedsMount")}
                      </td>
                      <td>
                        {storage.present && storage.filesystem && mountpoints.length === 0 && !alreadyPool ? (
                          <button
                            className="button compact primary"
                            type="button"
                            disabled={busy !== "" || !canManageStorage}
                            title={!canManageStorage ? u("filesStorageManageRequired") : undefined}
                            onClick={() => void mountFileStorage(storage)}
                          >
                            {busy === `storage-mount-${storage.device}` ? t("working") : t("mount")}
                          </button>
                        ) : "—"}
                      </td>
                    </tr>
                  );
                })}
                {effectiveFileStorage.length === 0 && (
                  <tr>
                    <td colSpan={7} className="muted">{t("fileNoAssignedStorage")}</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <p className="muted small">{t("fileAssignedStorageNotice")}</p>
        </Panel>
      )}

      {canManage && section === "storage" && canShowStorageInventory && (
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
              {mountOptions.length === 0 && (
                <div className="notice">
                  {effectiveFileStorage.length === 0 ? t("fileNoAssignedStorage") : t("fileAssignedStorageNeedsMount")}
                </div>
              )}
              <p className="muted small">{t("filePoolFoundationNotice")}</p>
              <button className="button primary" type="submit" disabled={busy !== "" || mountOptions.length === 0}>
                {busy === "pool" ? t("working") : t("fileCreatePool")}
              </button>
            </form>
          </Panel>
      )}
      {canManage && section === "folders" && (
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
                      {pool.name}
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
      )}
    </div>
  );
}

type FolderDeletionBlockers = {
  visible_entries: number;
  upload_artifacts: number;
  trash_items: number;
  trash_metadata: number;
  other_hidden: number;
};

function parseFolderDeletionBlockers(value: unknown): FolderDeletionBlockers | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const data = value as Record<string, unknown>;
  const count = (key: string): number | undefined => {
    const item = data[key];
    return typeof item === "number" && Number.isSafeInteger(item) && item >= 0 ? item : undefined;
  };
  const visible_entries = count("visible_entries");
  const upload_artifacts = count("upload_artifacts");
  const trash_items = count("trash_items");
  const trash_metadata = count("trash_metadata");
  const other_hidden = count("other_hidden");
  if ([visible_entries, upload_artifacts, trash_items, trash_metadata, other_hidden].some((v) => v === undefined)) return undefined;
  return {
    visible_entries: visible_entries!,
    upload_artifacts: upload_artifacts!,
    trash_items: trash_items!,
    trash_metadata: trash_metadata!,
    other_hidden: other_hidden!,
  };
}

type NASMountOption = {
  path: string;
  filesystem?: string;
  device?: string;
};

function localizeFileStorageError(
  reason: unknown,
  locale: "ru" | "en",
  fallback: string,
  mountFailed: string,
  mountedElsewhere: string,
  filesystemError: string,
): string {
  if (!(reason instanceof Error)) return fallback;
  const message = reason instanceof APIError ? reason.rawMessage : reason.message || "";
  if (locale !== "ru") return describeFileError(reason, locale, fallback);

  if (message.includes("device is already mounted at")) {
    const location = message.split("device is already mounted at")[1]?.trim();
    return location ? `${mountedElsewhere} ${location}` : mountedElsewhere;
  }
  if (message.includes("filesystem diagnostic:")) {
    return filesystemError;
  }
  if (
    message.includes("mount device") ||
    message.includes("wrong fs type") ||
    message.includes("bad superblock")
  ) {
    return mountFailed;
  }
  if (message.includes("system storage helper is outdated")) {
    return "Системный модуль управления хранилищем устарел. Установите последнюю версию Home-AI-Core.";
  }
  if (message.includes("storage helper")) {
    return "Не удалось связаться с системным модулем управления хранилищем.";
  }
  return describeFileError(reason, locale, fallback);
}

function fileStorageMountOptions(assignments: StoragePurposeAssignment[]): NASMountOption[] {
  const byPath = new Map<string, NASMountOption>();
  for (const storage of assignments) {
    if (!storage.present || !storage.filesystem) continue;
    for (const mountpoint of storage.mountpoints || []) {
      if (!mountpoint.startsWith("/mnt/home-ai-core/")) continue;
      byPath.set(mountpoint, {
        path: mountpoint,
        filesystem: storage.filesystem,
        device: storage.device,
      });
    }
  }
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

const fileUploadChunkBytes = 8 * 1024 * 1024;

async function verifyUploadedChunks(
  file: File,
  chunks: Array<{offset: number; size: number; sha256: string}>,
): Promise<boolean> {
  if (!globalThis.crypto?.subtle) return true;
  for (const chunk of chunks) {
    if (chunk.offset < 0 || chunk.size <= 0 || chunk.offset + chunk.size > file.size) {
      return false;
    }
    const digest = await sha256Blob(file.slice(chunk.offset, chunk.offset + chunk.size));
    if (digest && digest !== chunk.sha256) {
      return false;
    }
  }
  return true;
}

async function sha256Blob(blob: Blob): Promise<string | undefined> {
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) return undefined;
  try {
    const digest = await subtle.digest("SHA-256", await blob.arrayBuffer());
    return Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, "0")).join("");
  } catch {
    return undefined;
  }
}

