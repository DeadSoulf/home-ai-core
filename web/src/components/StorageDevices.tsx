import { Fragment, useState } from "react";
import { api } from "../api/client";
import type { BlockNode } from "../api/types";
import { useI18n } from "../i18n";

type Filesystem = "ext4" | "xfs" | "vfat";

function bytes(value = 0) {
  if (value >= 1024 ** 4) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 4) + " TiB";
  if (value >= 1024 ** 3) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
  if (value >= 1024 ** 2) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 2) + " MiB";
  if (value >= 1024) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024) + " KiB";
  return value + " B";
}

function flatten(
  nodes: BlockNode[],
  collapsed: string[],
  depth = 0,
): Array<{node: BlockNode; depth: number}> {
  return nodes.flatMap((node) => {
    const key = node.path || node.name;
    const hideChildren = node.type === "disk" && collapsed.includes(key);
    return [
      {node, depth},
      ...(hideChildren ? [] : flatten(node.children || [], collapsed, depth + 1)),
    ];
  });
}

export function StorageDevices({
  devices,
  onChanged,
}: {
  devices: BlockNode[];
  onChanged: () => void;
}) {
  const {t} = useI18n();
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [formatDevice, setFormatDevice] = useState("");
  const [createDisk, setCreateDisk] = useState("");
  const [partitionSizeGiB, setPartitionSizeGiB] = useState("");
  const [filesystem, setFilesystem] = useState<Filesystem>("ext4");
  const [label, setLabel] = useState("");
  const [collapsed, setCollapsed] = useState<string[]>([]);

  function toggleDisk(node: BlockNode) {
    const key = node.path || node.name;
    setCollapsed((current) =>
      current.includes(key)
        ? current.filter((item) => item !== key)
        : [...current, key],
    );
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
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
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
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function format(node: BlockNode) {
    if (!node.path) return;
    const confirmation = `FORMAT ${node.path}`;
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
        await api.storageOperation({
          operation: "unmount",
          device: node.path,
        });
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
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
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
    }

    const confirmation = `CREATE ${node.path}`;
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
    try {
      const result = await api.storageOperation({
        operation: "partition.create",
        device: node.path,
        size_mib: sizeMiB || undefined,
        confirm: confirmation,
      });
      setMessage(result.message);
      setCreateDisk("");
      setPartitionSizeGiB("");
      onChanged();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function deleteAllPartitions(node: BlockNode) {
    if (!node.path) return;
    const confirmation = `DELETE ALL ${node.path}`;
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
    try {
      const result = await api.storageOperation({
        operation: "partition.delete_all",
        device: node.path,
        confirm: confirmation,
      });
      setMessage(result.message);
      onChanged();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function deletePartition(node: BlockNode) {
    if (!node.path) return;
    const confirmation = `DELETE ${node.path}`;
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
    try {
      const result = await api.storageOperation({
        operation: "partition.delete",
        device: node.path,
        confirm: confirmation,
      });
      setMessage(result.message);
      onChanged();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  if (devices.length === 0) {
    return <div className="empty-state">{t("noBlockDevices")}</div>;
  }

  const rows = flatten(devices, collapsed);

  return (
    <div className="storage-tree-wrap">
      {error && <div className="form-error">{error}</div>}
      {message && <div className="storage-success">{message}</div>}

      <div className="table-wrap">
        <table className="storage-tree-table">
          <thead>
            <tr>
              <th>NAME</th>
              <th>TYPE</th>
              <th>FSTYPE</th>
              <th>SIZE</th>
              <th>MOUNTPOINTS</th>
              <th>PKNAME</th>
              <th>{t("actions")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({node, depth}, index) => {
              const mounted = node.mountpoints.length > 0;
              const root = node.mountpoints.includes("/");
              const operationBusy = !!node.path && busy === node.path;
              const formatting = !!node.path && formatDevice === node.path;
              const creating = !!node.path && createDisk === node.path;
              const collapseKey = node.path || node.name;
              const isCollapsed = collapsed.includes(collapseKey);
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
                !node.system;
              const deletable =
                !!node.path &&
                node.type === "part" &&
                !node.system;
              const canCreatePartition =
                !!node.path &&
                node.type === "disk" &&
                !node.system;
              const renameable =
                !!node.path &&
                !mounted &&
                !node.system &&
                ["ext4", "xfs", "vfat", "fat", "fat32"].includes((node.filesystem || "").toLowerCase());

              return (
                <Fragment key={`${node.path || node.name}-${index}`}>
                  <tr className={node.system ? "storage-system-row" : ""}>
                    <td>
                      <div
                        className="storage-tree-name"
                        style={{paddingLeft: `${depth * 22}px`}}
                        title={[node.vendor, node.model, node.serial].filter(Boolean).join(" · ")}
                      >
                        {node.type === "disk" && (node.children?.length || 0) > 0 ? (
                          <button
                            type="button"
                            className="storage-collapse-button"
                            onClick={() => toggleDisk(node)}
                            title={isCollapsed ? t("expandPartitions") : t("collapsePartitions")}
                          >
                            {isCollapsed ? "▶" : "▼"}
                          </button>
                        ) : depth > 0 ? (
                          <span className="storage-tree-branch">↳</span>
                        ) : (
                          <span className="storage-tree-spacer" />
                        )}
                        <strong className="mono">{node.name}</strong>
                        {node.system && <span className="status-badge storage-system">{t("systemDisk")}</span>}
                        {node.label && <span className="storage-inline-label">{node.label}</span>}
                      </div>
                      {depth === 0 && (node.vendor || node.model) && (
                        <div className="storage-tree-model">
                          {[node.vendor, node.model].filter(Boolean).join(" ")}
                        </div>
                      )}
                    </td>
                    <td>{node.type || "—"}</td>
                    <td>{node.filesystem || "—"}</td>
                    <td className="mono">{bytes(node.size_bytes)}</td>
                    <td className="mono">{node.mountpoints.join(", ") || "—"}</td>
                    <td className="mono">{node.parent_name || "—"}</td>
                    <td>
                      <div className="storage-tree-actions">
                        {mounted && mountable && (
                          <button
                            type="button"
                            className="button secondary compact"
                            disabled={operationBusy || root}
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
                        {node.type === "disk" && (
                          <>
                            <button
                              type="button"
                              className="button secondary compact"
                              disabled={operationBusy || !canCreatePartition}
                              onClick={() => {
                                setCreateDisk(creating ? "" : (node.path || ""));
                                setFormatDevice("");
                                setError("");
                              }}
                            >
                              {t("createPartition")}
                            </button>
                            <button
                              type="button"
                              className="button danger compact"
                              disabled={operationBusy || node.system || (node.children?.length || 0) === 0}
                              onClick={() => deleteAllPartitions(node)}
                            >
                              {t("deleteAllPartitions")}
                            </button>
                          </>
                        )}
                        {node.type === "part" && (
                          <>
                            <button
                              type="button"
                              className="button danger compact"
                              disabled={operationBusy || !formattable}
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
                              onClick={() => deletePartition(node)}
                            >
                              {t("deletePartition")}
                            </button>
                          </>
                        )}
                        {!mountable && !renameable && node.type !== "part" && node.type !== "disk" && <span className="muted">—</span>}
                      </div>
                    </td>
                  </tr>

                  {creating && (
                    <tr className="storage-format-row">
                      <td colSpan={7}>
                        <div className="storage-format">
                          <div>
                            <label>
                              {t("partitionSizeGiB")}
                              <input
                                inputMode="decimal"
                                value={partitionSizeGiB}
                                onChange={(event) => setPartitionSizeGiB(event.target.value)}
                                placeholder={t("allRemainingSpace")}
                              />
                            </label>
                          </div>
                          <div className="storage-format-warning">{t("createPartitionWarning")}</div>
                          <div className="storage-format-actions">
                            <button
                              type="button"
                              className="button secondary"
                              disabled={operationBusy}
                              onClick={() => {
                                setCreateDisk("");
                                setPartitionSizeGiB("");
                              }}
                            >
                              {t("cancel")}
                            </button>
                            <button
                              type="button"
                              className="button primary"
                              disabled={operationBusy}
                              onClick={() => createPartition(node)}
                            >
                              {operationBusy ? t("working") : t("createPartition")}
                            </button>
                          </div>
                        </div>
                      </td>
                    </tr>
                  )}

                  {formatting && (
                    <tr className="storage-format-row">
                      <td colSpan={7}>
                        <div className="storage-format">
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
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </div>

      {devices.some((disk) => disk.system) && (
        <div className="storage-protection">{t("systemDiskProtection")}</div>
      )}
    </div>
  );
}
