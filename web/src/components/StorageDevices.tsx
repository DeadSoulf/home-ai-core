import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { BlockNode, StoragePurpose, StoragePurposeAssignment } from "../api/types";
import { useI18n } from "../i18n";

type Filesystem = "ext4" | "xfs" | "vfat";

function bytes(value = 0) {
  if (value >= 1024 ** 4) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 4) + " TiB";
  if (value >= 1024 ** 3) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
  if (value >= 1024 ** 2) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 2) + " MiB";
  if (value >= 1024) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024) + " KiB";
  return value + " B";
}

function nodeKey(node: BlockNode) {
  return node.path || node.name;
}

function descendants(nodes: BlockNode[], depth = 0): Array<{node: BlockNode; depth: number}> {
  return nodes.flatMap((node) => [
    {node, depth},
    ...descendants(node.children || [], depth + 1),
  ]);
}

function diskUsagePercent(node: BlockNode): number | undefined {
  const size = node.size_bytes || 0;
  if (size <= 0) return undefined;
  if (node.free_known && node.free_bytes !== undefined) {
    return Math.max(0, Math.min(100, ((size - node.free_bytes) / size) * 100));
  }
  if (node.unallocated_bytes !== undefined) {
    return Math.max(0, Math.min(100, ((size - node.unallocated_bytes) / size) * 100));
  }
  return undefined;
}

export function StorageDevices({
  devices,
  onChanged,
  canManage = false,
}: {
  devices: BlockNode[];
  onChanged: () => void;
  canManage?: boolean;
}) {
  const {t} = useI18n();
  const disks = devices.filter((node) => node.type === "disk");
  const [selectedDiskKey, setSelectedDiskKey] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [formatDevice, setFormatDevice] = useState("");
  const [createDisk, setCreateDisk] = useState("");
  const [partitionSizeGiB, setPartitionSizeGiB] = useState("");
  const [filesystem, setFilesystem] = useState<Filesystem>("ext4");
  const [label, setLabel] = useState("");
  const [diskName, setDiskName] = useState("");
  const [partitionPurpose, setPartitionPurpose] = useState<StoragePurpose>("files");
  const [purposes, setPurposes] = useState<StoragePurposeAssignment[]>([]);
  const [partitionProgress, setPartitionProgress] = useState<{device: string; text: string} | null>(null);

  const selectedDisk = disks.find((disk) => nodeKey(disk) === selectedDiskKey);

  useEffect(() => {
    if (selectedDiskKey && !devices.some((node) => nodeKey(node) === selectedDiskKey)) {
      setSelectedDiskKey("");
    }
  }, [devices, selectedDiskKey]);

  useEffect(() => {
    if (!selectedDisk) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeDisk();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [selectedDiskKey]);

  async function refreshPurposes() {
    if (!canManage) {
      setPurposes([]);
      return;
    }
    try {
      setPurposes(await api.storagePurposes());
    } catch (reason) {
      setStorageError(reason);
    }
  }

  useEffect(() => {
    void refreshPurposes();
  }, [canManage]);

  function openDisk(node: BlockNode) {
    setSelectedDiskKey(nodeKey(node));
    setFormatDevice("");
    setCreateDisk("");
    setError("");
    setMessage("");
  }

  function closeDisk() {
    setSelectedDiskKey("");
    setFormatDevice("");
    setCreateDisk("");
    setPartitionSizeGiB("");
    setDiskName("");
    setLabel("");
    setError("");
  }

  function purposeForNode(node: BlockNode): StoragePurposeAssignment | undefined {
    if (node.uuid) {
      const byUUID = purposes.find((item) => item.filesystem_uuid === node.uuid);
      if (byUUID) return byUUID;
    }
    return purposes.find((item) => item.device === node.path);
  }

  function nodeOrDescendantInUse(node: BlockNode): boolean {
    if (purposeForNode(node)?.in_use) return true;
    return (node.children || []).some((child) => nodeOrDescendantInUse(child));
  }

  function setStorageError(reason: unknown) {
    const detail = reason instanceof Error ? reason.message : t("requestFailed");
    const normalized = detail.toLowerCase();
    let friendly = "";
    if (normalized.includes("used by a home-ai file pool")) friendly = t("storageInUseBlocked");
    else if (normalized.includes("system disk")) friendly = t("storageErrorSystemDisk");
    else if (normalized.includes("deactivate lvm") || normalized.includes("volume group")) friendly = t("storageErrorLvmBusy");
    else if (normalized.includes("swap")) friendly = t("storageErrorSwap");
    else if (normalized.includes("kernel could not reload") || normalized.includes("still reports partition")) friendly = t("storageErrorKernelReload");
    else if (normalized.includes("still mounted") || normalized.includes("unmount")) friendly = t("storageErrorUnmount");
    else if (normalized.includes("exceeds available") || normalized.includes("no space")) friendly = t("storageErrorNoSpace");
    else if (normalized.includes("filesystem verification") || normalized.includes("format device")) friendly = t("storageErrorFormat");
    setError(friendly ? friendly + "\n" + t("technicalDetails") + ": " + detail : detail);
  }

  async function changePurpose(node: BlockNode, purpose: "" | StoragePurpose) {
    if (!node.path) return;
    setBusy(node.path);
    setError("");
    setMessage("");
    try {
      await api.setStoragePurpose(node.path, purpose || undefined);
      setMessage(purpose ? t("storagePurposeSaved") : t("storagePurposeCleared"));
      await refreshPurposes();
    } catch (reason) {
      setStorageError(reason);
    } finally {
      setBusy("");
    }
  }

  async function perform(operation: "mount" | "unmount", node: BlockNode) {
    if (!node.path) return;
    setBusy(node.path);
    setError("");
    setMessage("");
    try {
      const result = await api.storageOperation({operation, device: node.path});
      setMessage(result.message);
      onChanged();
    } catch (reason) {
      setStorageError(reason);
    } finally {
      setBusy("");
    }
  }

  async function renamePhysicalDisk(node: BlockNode) {
    if (!node.path || node.type !== "disk") return;
    const value = window.prompt(
      t("diskDisplayNamePrompt").replace("{device}", node.path),
      node.display_name || "",
    );
    if (value === null) return;
    const nextName = value.trim();
    if (!nextName) {
      setError(t("diskNameRequired"));
      return;
    }

    setBusy(node.path);
    setError("");
    setMessage("");
    try {
      await api.setDiskName(node.path, nextName);
      setMessage(t("diskNameSaved"));
      onChanged();
    } catch (reason) {
      setStorageError(reason);
    } finally {
      setBusy("");
    }
  }

  async function renameFilesystem(node: BlockNode) {
    if (!node.path) return;
    const value = window.prompt(t("renameDiskPrompt").replace("{device}", node.path), node.label || "");
    if (value === null) return;
    const nextLabel = value.trim();
    if (!nextLabel) {
      setError(t("diskNameRequired"));
      return;
    }

    setBusy(node.path);
    setError("");
    setMessage("");
    try {
      const result = await api.storageOperation({
        operation: "label.rename",
        device: node.path,
        label: nextLabel,
      });
      setMessage(result.message);
      onChanged();
    } catch (reason) {
      setStorageError(reason);
    } finally {
      setBusy("");
    }
  }

  async function format(node: BlockNode) {
    if (!node.path) return;
    const confirmation = "FORMAT " + node.path;
    const typed = window.prompt(
      t("formatTypeConfirmation")
        .replace("{device}", node.path)
        .replace("{confirmation}", confirmation),
      "",
    );
    if (typed !== confirmation) {
      if (typed !== null) setError(t("formatConfirmationMismatch"));
      return;
    }

    setBusy(node.path);
    setError("");
    setMessage("");
    try {
      if (node.mountpoints.length > 0) {
        await api.storageOperation({operation: "unmount", device: node.path});
      }
      const result = await api.storageOperation({
        operation: "format",
        device: node.path,
        filesystem,
        label: label.trim(),
        confirm: confirmation,
      });
      setMessage(result.message);
      setFormatDevice("");
      setLabel("");
      onChanged();
      await refreshPurposes();
    } catch (reason) {
      setStorageError(reason);
    } finally {
      setBusy("");
    }
  }

  async function createPartition(node: BlockNode) {
    if (!node.path) return;

    let sizeMiB = 0;
    const sizeText = partitionSizeGiB.trim();
    if (sizeText !== "") {
      const sizeGiB = Number(sizeText.replace(",", "."));
      if (!Number.isFinite(sizeGiB) || sizeGiB <= 0) {
        setError(t("invalidPartitionSize"));
        return;
      }
      sizeMiB = Math.round(sizeGiB * 1024);
      const availableBytes = Math.max(0, (node.unallocated_bytes || 0) - 4 * 1024 * 1024);
      if (node.unallocated_bytes && sizeMiB * 1024 * 1024 > availableBytes) {
        setError(t("partitionSizeExceedsAvailable").replace("{available}", bytes(availableBytes)));
        return;
      }
    }

    const confirmation = "CREATE " + node.path;
    const typed = window.prompt(
      t("createPartitionConfirmation")
        .replace("{device}", node.path)
        .replace("{confirmation}", confirmation),
      "",
    );
    if (typed !== confirmation) {
      if (typed !== null) setError(t("partitionConfirmationMismatch"));
      return;
    }

    setBusy(node.path);
    setError("");
    setMessage("");
    setPartitionProgress({device: node.path, text: t("partitionCreating")});
    try {
      const result = await api.storageOperation({
        operation: "partition.create",
        device: node.path,
        size_mib: sizeMiB || undefined,
        purpose: partitionPurpose,
        confirm: confirmation,
      });
      if (diskName.trim()) {
        await api.setDiskName(node.path, diskName.trim());
      }
      setPartitionProgress({device: node.path, text: t("partitionRefreshing")});
      setMessage(result.warning ? result.message + " · " + result.warning : result.message);
      setCreateDisk("");
      setPartitionSizeGiB("");
      setDiskName("");
      setPartitionPurpose("files");
      onChanged();
      await refreshPurposes();
      setPartitionProgress({device: node.path, text: t("partitionCreated")});
      window.setTimeout(() => setPartitionProgress(null), 1200);
    } catch (reason) {
      setPartitionProgress(null);
      setStorageError(reason);
    } finally {
      setBusy("");
    }
  }

  async function deleteAllPartitions(node: BlockNode) {
    if (!node.path) return;
    const confirmation = node.name;
    const typed = window.prompt(
      t("deleteAllPartitionsConfirmation")
        .replace("{device}", node.path)
        .replace("{confirmation}", confirmation),
      "",
    );
    if (typed !== confirmation) {
      if (typed !== null) setError(t("partitionConfirmationMismatch"));
      return;
    }

    setBusy(node.path);
    setError("");
    setMessage("");
    setPartitionProgress({device: node.path, text: t("partitionsDeletingAll")});
    try {
      const result = await api.storageOperation({
        operation: "partition.delete_all",
        device: node.path,
        confirm: confirmation,
      });
      setPartitionProgress({device: node.path, text: t("partitionRefreshing")});
      setMessage(result.message);
      onChanged();
      await refreshPurposes();
      setPartitionProgress({device: node.path, text: t("partitionsDeletedAll")});
      window.setTimeout(() => setPartitionProgress(null), 1200);
    } catch (reason) {
      setPartitionProgress(null);
      setStorageError(reason);
    } finally {
      setBusy("");
    }
  }

  async function deletePartition(node: BlockNode) {
    if (!node.path) return;
    const confirmation = node.name;
    const typed = window.prompt(
      t("deletePartitionConfirmation")
        .replace("{device}", node.path)
        .replace("{confirmation}", confirmation),
      "",
    );
    if (typed !== confirmation) {
      if (typed !== null) setError(t("partitionConfirmationMismatch"));
      return;
    }

    setBusy(node.path);
    setError("");
    setMessage("");
    setPartitionProgress({device: node.path, text: t("partitionDeleting")});
    try {
      const result = await api.storageOperation({
        operation: "partition.delete",
        device: node.path,
        confirm: confirmation,
      });
      setPartitionProgress({device: node.path, text: t("partitionRefreshing")});
      setMessage(result.message);
      onChanged();
      await refreshPurposes();
      setPartitionProgress({device: node.path, text: t("partitionDeleted")});
      window.setTimeout(() => setPartitionProgress(null), 1200);
    } catch (reason) {
      setPartitionProgress(null);
      setStorageError(reason);
    } finally {
      setBusy("");
    }
  }

  if (disks.length === 0) {
    return <div className="empty-state">{t("noBlockDevices")}</div>;
  }

  const selectedChildren = selectedDisk ? descendants(selectedDisk.children || []) : [];

  return (
    <div className="storage-disks">
      {error && <div className="form-error">{error}</div>}
      {message && <div className="storage-success">{message}</div>}
      {partitionProgress && (
        <div className="storage-operation-progress" role="status" aria-live="polite">
          <div className="storage-operation-progress-head">
            <strong>{partitionProgress.text}</strong>
            <span className="mono">{partitionProgress.device}</span>
          </div>
          <div className="storage-operation-progress-track">
            <div className="storage-operation-progress-bar" />
          </div>
        </div>
      )}

      <div className="storage-disk-grid">
        {disks.map((disk) => {
          const usage = diskUsagePercent(disk);
          const remaining = disk.free_known
            ? bytes(disk.free_bytes || 0)
            : disk.unallocated_bytes !== undefined
              ? bytes(disk.unallocated_bytes)
              : "—";
          const remainingLabel = disk.free_known ? t("freeSpace") : t("unallocated");
          const purposeCount = descendants(disk.children || []).filter((item) => purposeForNode(item.node)).length;

          return (
            <button
              type="button"
              className={"storage-disk-card" + (selectedDiskKey === nodeKey(disk) ? " selected" : "")}
              key={nodeKey(disk)}
              onClick={() => openDisk(disk)}
            >
              <div className="storage-disk-card-head">
                <span className="storage-disk-icon" aria-hidden="true">▰</span>
                <span className="storage-disk-badges">
                  {disk.system && <span className="status-badge storage-system">{t("systemDisk")}</span>}
                  {disk.transport && <span className="storage-inline-label">{disk.transport.toUpperCase()}</span>}
                </span>
              </div>
              <span className="storage-disk-name">{disk.display_name || disk.name}</span>
              <span className="storage-disk-model">
                {[disk.vendor, disk.model].filter(Boolean).join(" ") || disk.path || disk.name}
              </span>
              <span className="storage-disk-capacity-row">
                <span><small>{t("size")}</small><strong>{bytes(disk.size_bytes)}</strong></span>
                <span><small>{remainingLabel}</small><strong>{remaining}</strong></span>
              </span>
              <span className="storage-disk-usage" aria-hidden="true">
                <span style={{width: usage === undefined ? "0%" : usage.toFixed(1) + "%"}} />
              </span>
              <span className="storage-disk-card-foot">
                <span>{(disk.children || []).length} · {t("diskTypePartition")}</span>
                {purposeCount > 0 && <span>{purposeCount} · {t("storagePurpose")}</span>}
                {disk.temperature_c !== undefined && <span>{disk.temperature_c}°C</span>}
              </span>
            </button>
          );
        })}
      </div>

      {devices.some((disk) => disk.system) && (
        <div className="storage-protection">{t("systemDiskProtection")}</div>
      )}

      {selectedDisk && (
        <>
          <button
            type="button"
            className="storage-drawer-backdrop"
            aria-label={t("cancel")}
            onClick={closeDisk}
          />
          <aside className="storage-drawer" aria-label={selectedDisk.display_name || selectedDisk.name}>
            <div className="storage-drawer-head">
              <div>
                <div className="storage-drawer-title-row">
                  <h3>{selectedDisk.display_name || selectedDisk.name}</h3>
                  {selectedDisk.system && <span className="status-badge storage-system">{t("systemDisk")}</span>}
                </div>
                <div className="mono storage-drawer-path">{selectedDisk.path || selectedDisk.name}</div>
              </div>
              <button type="button" className="storage-drawer-close" onClick={closeDisk} aria-label={t("cancel")}>×</button>
            </div>

            <div className="storage-drawer-scroll">
              <section className="storage-drawer-section">
                <dl className="storage-disk-details">
                  <div><dt>{t("size")}</dt><dd>{bytes(selectedDisk.size_bytes)}</dd></div>
                  <div><dt>{t("partitionTable")}</dt><dd>{(selectedDisk.partition_table || "—").toUpperCase().replace("DOS", "MBR")}</dd></div>
                  <div><dt>{t("model")}</dt><dd>{[selectedDisk.vendor, selectedDisk.model].filter(Boolean).join(" ") || "—"}</dd></div>
                  <div><dt>{t("type")}</dt><dd>{selectedDisk.transport ? selectedDisk.transport.toUpperCase() : t("diskTypeDisk")}</dd></div>
                  <div><dt>{t("unallocated")}</dt><dd>{bytes(selectedDisk.unallocated_bytes || 0)}</dd></div>
                  <div><dt>{t("state")}</dt><dd>{selectedDisk.health || "—"}</dd></div>
                </dl>
                <details className="technical-details">
                  <summary>{t("technicalDetails")}</summary>
                  <dl className="details">
                    <dt>{t("deviceId")}</dt><dd className="mono">{selectedDisk.serial || selectedDisk.name}</dd>
                    <dt>{t("mountPoints")}</dt><dd className="mono">{selectedDisk.mountpoints.join(", ") || "—"}</dd>
                    <dt>{t("diskHealth")}</dt><dd>{selectedDisk.health || "—"}</dd>
                    <dt>{t("temperature")}</dt><dd>{selectedDisk.temperature_c === undefined ? "—" : selectedDisk.temperature_c + "°C"}</dd>
                  </dl>
                </details>
              </section>

              {canManage && (
                <section className="storage-drawer-section">
                  <h4>{t("actions")}</h4>
                  <div className="storage-drawer-actions">
                    <button
                      type="button"
                      className="button secondary"
                      disabled={busy === selectedDisk.path}
                      onClick={() => renamePhysicalDisk(selectedDisk)}
                    >
                      {t("renamePhysicalDisk")}
                    </button>
                    <button
                      type="button"
                      className="button primary"
                      disabled={busy === selectedDisk.path || selectedDisk.system}
                      onClick={() => {
                        const opening = createDisk !== selectedDisk.path;
                        setCreateDisk(opening ? (selectedDisk.path || "") : "");
                        setDiskName(opening ? (selectedDisk.display_name || "") : "");
                        setFormatDevice("");
                        setError("");
                      }}
                    >
                      {t("createPartition")}
                    </button>
                    <button
                      type="button"
                      className="button danger"
                      disabled={
                        busy === selectedDisk.path ||
                        selectedDisk.system ||
                        nodeOrDescendantInUse(selectedDisk) ||
                        (selectedDisk.children?.length || 0) === 0
                      }
                      title={nodeOrDescendantInUse(selectedDisk) ? t("storageInUseBlocked") : undefined}
                      onClick={() => deleteAllPartitions(selectedDisk)}
                    >
                      {t("deleteAllPartitions")}
                    </button>
                  </div>

                  {createDisk === selectedDisk.path && (
                    <div className="storage-format storage-drawer-form">
                      <div>
                        <label>
                          {t("diskDisplayName")}
                          <input
                            value={diskName}
                            maxLength={64}
                            onChange={(event) => setDiskName(event.target.value)}
                            placeholder={t("diskDisplayNamePlaceholder")}
                          />
                        </label>
                        <label>
                          {t("partitionSizeGiB")}
                          <input
                            inputMode="decimal"
                            value={partitionSizeGiB}
                            onChange={(event) => setPartitionSizeGiB(event.target.value)}
                            placeholder={t("allRemainingSpace")}
                          />
                        </label>
                        <label>
                          {t("storagePurpose")}
                          <select
                            value={partitionPurpose}
                            onChange={(event) => setPartitionPurpose(event.target.value as StoragePurpose)}
                          >
                            <option value="files">{t("storagePurposeFiles")}</option>
                            <option value="video">{t("storagePurposeVideo")}</option>
                          </select>
                        </label>
                      </div>
                      <div className="storage-capacity-summary">
                        <strong>{t("unallocated")}:</strong> {bytes(selectedDisk.unallocated_bytes || 0)}
                      </div>
                      <div className="storage-format-warning">{t("createPartitionWarning")}</div>
                      <div className="storage-format-actions">
                        <button
                          type="button"
                          className="button secondary"
                          disabled={busy === selectedDisk.path}
                          onClick={() => {
                            setCreateDisk("");
                            setPartitionSizeGiB("");
                            setDiskName("");
                            setPartitionPurpose("files");
                          }}
                        >
                          {t("cancel")}
                        </button>
                        <button
                          type="button"
                          className="button primary"
                          disabled={busy === selectedDisk.path}
                          onClick={() => createPartition(selectedDisk)}
                        >
                          {busy === selectedDisk.path ? t("working") : t("createPartition")}
                        </button>
                      </div>
                    </div>
                  )}
                </section>
              )}

              <section className="storage-drawer-section">
                <div className="storage-drawer-section-head">
                  <h4>{t("diskTypePartition")}</h4>
                  <span className="status-badge">{selectedChildren.length}</span>
                </div>

                <div className="storage-partition-list">
                  {selectedChildren.map(({node, depth}) => {
                    const mounted = node.mountpoints.length > 0;
                    const root = node.mountpoints.includes("/");
                    const operationBusy = !!node.path && busy === node.path;
                    const purposeAssignment = purposeForNode(node);
                    const storageInUse = purposeAssignment?.in_use === true;
                    const mountable =
                      !!node.path &&
                      !!node.filesystem &&
                      node.filesystem !== "swap" &&
                      node.filesystem !== "LVM2_member" &&
                      node.type !== "disk" &&
                      node.type !== "rom";
                    const formattable =
                      !!node.path &&
                      node.type === "part" &&
                      !node.system &&
                      !storageInUse;
                    const deletable = formattable;
                    const renameable =
                      !!node.path &&
                      !mounted &&
                      !node.system &&
                      ["ext4", "xfs", "vfat", "fat", "fat32"].includes((node.filesystem || "").toLowerCase());
                    const formatting = !!node.path && formatDevice === node.path;

                    return (
                      <article className="storage-partition-card" key={nodeKey(node)} style={{marginLeft: Math.min(depth, 2) * 12}}>
                        <div className="storage-partition-head">
                          <div>
                            <strong>{node.label || node.display_name || node.name}</strong>
                            <span className="mono">{node.path || node.name}</span>
                          </div>
                          <div className="storage-disk-badges">
                            {node.filesystem && <span className="storage-inline-label">{node.filesystem}</span>}
                            {purposeAssignment && (
                              <span className="status-badge">
                                {purposeAssignment.purpose === "files" ? t("storagePurposeFiles") : t("storagePurposeVideo")}
                              </span>
                            )}
                            {storageInUse && <span className="status-badge status-success">{t("storageInUseFilePool")}</span>}
                          </div>
                        </div>

                        <dl className="storage-partition-meta">
                          <div><dt>{t("size")}</dt><dd>{bytes(node.size_bytes)}</dd></div>
                          <div><dt>{t("freeSpace")}</dt><dd>{node.free_known ? bytes(node.free_bytes || 0) : "—"}</dd></div>
                          <div><dt>{t("mountPoints")}</dt><dd className="mono">{node.mountpoints.join(", ") || "—"}</dd></div>
                          {node.type === "lvm" && <div><dt>{t("type")}</dt><dd>LVM</dd></div>}
                        </dl>

                        {canManage && (
                          <div className="storage-partition-actions">
                            {mounted && mountable && (
                              <button
                                type="button"
                                className="button secondary compact"
                                disabled={operationBusy || root || storageInUse}
                                title={storageInUse ? t("storageInUseBlocked") : undefined}
                                onClick={() => perform("unmount", node)}
                              >
                                {operationBusy ? t("working") : t("unmount")}
                              </button>
                            )}
                            {!mounted && mountable && (
                              <button
                                type="button"
                                className="button secondary compact"
                                disabled={operationBusy}
                                onClick={() => perform("mount", node)}
                              >
                                {operationBusy ? t("working") : t("mount")}
                              </button>
                            )}
                            {renameable && (
                              <button
                                type="button"
                                className="button secondary compact"
                                disabled={operationBusy}
                                onClick={() => renameFilesystem(node)}
                              >
                                {t("renameDisk")}
                              </button>
                            )}
                            {(node.type === "part" || node.type === "lvm") && !node.system && (
                              <label className="storage-purpose-control">
                                <span>{t("storagePurpose")}</span>
                                <select
                                  value={purposeAssignment?.purpose || ""}
                                  disabled={operationBusy || storageInUse}
                                  title={storageInUse ? t("storageInUseBlocked") : undefined}
                                  onChange={(event) => void changePurpose(node, event.target.value as "" | StoragePurpose)}
                                >
                                  <option value="">{t("storagePurposeNone")}</option>
                                  <option value="files">{t("storagePurposeFiles")}</option>
                                  <option value="video">{t("storagePurposeVideo")}</option>
                                </select>
                              </label>
                            )}
                            {node.type === "part" && (
                              <>
                                <button
                                  type="button"
                                  className="button danger compact"
                                  disabled={operationBusy || !formattable}
                                  title={storageInUse ? t("storageInUseBlocked") : undefined}
                                  onClick={() => {
                                    setFormatDevice(formatting ? "" : (node.path || ""));
                                    setCreateDisk("");
                                    setError("");
                                  }}
                                >
                                  {t("format")}
                                </button>
                                <button
                                  type="button"
                                  className="button danger compact"
                                  disabled={operationBusy || !deletable}
                                  title={storageInUse ? t("storageInUseBlocked") : undefined}
                                  onClick={() => deletePartition(node)}
                                >
                                  {t("deletePartition")}
                                </button>
                              </>
                            )}
                          </div>
                        )}

                        {canManage && formatting && (
                          <div className="storage-format storage-drawer-form">
                            <div>
                              <label>
                                {t("filesystem")}
                                <select value={filesystem} onChange={(event) => setFilesystem(event.target.value as Filesystem)}>
                                  <option value="ext4">ext4</option>
                                  <option value="xfs">xfs</option>
                                  <option value="vfat">FAT32 / vfat</option>
                                </select>
                              </label>
                              <label>
                                {t("volumeLabel")}
                                <input
                                  value={label}
                                  maxLength={32}
                                  onChange={(event) => setLabel(event.target.value)}
                                  placeholder={t("optional")}
                                />
                              </label>
                            </div>
                            <div className="storage-format-warning">{t("formatWarning")}</div>
                            <div className="storage-format-actions">
                              <button
                                type="button"
                                className="button secondary"
                                disabled={operationBusy}
                                onClick={() => setFormatDevice("")}
                              >
                                {t("cancel")}
                              </button>
                              <button
                                type="button"
                                className="button danger"
                                disabled={operationBusy}
                                onClick={() => format(node)}
                              >
                                {operationBusy ? t("working") : t("format")}
                              </button>
                            </div>
                          </div>
                        )}
                      </article>
                    );
                  })}
                  {selectedChildren.length === 0 && <div className="empty-state">—</div>}
                </div>
              </section>
            </div>
          </aside>
        </>
      )}
    </div>
  );
}
