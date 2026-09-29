import { useState } from "react";
import { api } from "../api/client";
import type { SystemResponse } from "../api/types";
import { useI18n } from "../i18n";

type BlockDevice = SystemResponse["system"]["block_devices"][number];
type Partition = BlockDevice["partitions"][number];
type Filesystem = "ext4" | "xfs" | "vfat";

function bytes(value = 0) {
  if (value >= 1024 ** 4) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 4) + " TiB";
  if (value >= 1024 ** 3) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
  if (value >= 1024 ** 2) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 2) + " MiB";
  return value + " B";
}

export function StorageDevices({
  devices,
  onChanged,
}: {
  devices: BlockDevice[];
  onChanged: () => void;
}) {
  const {t} = useI18n();
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [formatDevice, setFormatDevice] = useState("");
  const [filesystem, setFilesystem] = useState<Filesystem>("ext4");
  const [label, setLabel] = useState("");

  async function perform(
    operation: "mount" | "unmount",
    partition: Partition,
  ) {
    setBusy(partition.path);
    setError("");
    setMessage("");
    try {
      const result = await api.storageOperation({
        operation,
        device: partition.path,
      });
      setMessage(result.message);
      onChanged();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function format(partition: Partition) {
    const confirmation = `FORMAT ${partition.path}`;
    const typed = window.prompt(
      t("formatTypeConfirmation")
        .replace("{device}", partition.path)
        .replace("{confirmation}", confirmation),
      "",
    );
    if (typed !== confirmation) {
      if (typed !== null) setError(t("formatConfirmationMismatch"));
      return;
    }

    setBusy(partition.path);
    setError("");
    setMessage("");
    try {
      const result = await api.storageOperation({
        operation: "format",
        device: partition.path,
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

  if (devices.length === 0) {
    return <div className="empty-state">{t("noBlockDevices")}</div>;
  }

  return (
    <div className="storage-stack">
      {error && <div className="form-error">{error}</div>}
      {message && <div className="storage-success">{message}</div>}

      {devices.map((disk) => (
        <section className="storage-card" key={disk.name}>
          <div className="storage-card-header">
            <div>
              <div className="storage-title-row">
                <strong>{[disk.vendor, disk.model].filter(Boolean).join(" ") || disk.name}</strong>
                {disk.system && <span className="status-badge storage-system">{t("systemDisk")}</span>}
                {disk.removable && <span className="status-badge">{t("removable")}</span>}
                <span className="status-badge">{disk.rotational ? "HDD" : "SSD / Flash"}</span>
              </div>
              <div className="storage-path mono">{disk.path}</div>
            </div>
            <div className="storage-size">{bytes(disk.size_bytes)}</div>
          </div>

          <dl className="storage-meta">
            <div><dt>{t("serial")}</dt><dd className="mono">{disk.serial || "—"}</dd></div>
            <div><dt>{t("device")}</dt><dd className="mono">{disk.major_minor || disk.name}</dd></div>
            <div><dt>{t("partitions")}</dt><dd>{disk.partitions.length}</dd></div>
          </dl>

          <div className="storage-partitions">
            {disk.partitions.length === 0 && (
              <div className="storage-empty-partitions">{t("noPartitions")}</div>
            )}

            {disk.partitions.map((part) => {
              const mounted = part.mountpoints.length > 0;
              const root = part.mountpoints.includes("/");
              const formatting = formatDevice === part.path;
              const operationBusy = busy === part.path;

              return (
                <div className="storage-partition" key={part.name}>
                  <div className="storage-partition-main">
                    <div className="storage-partition-name">
                      <strong className="mono">{part.path}</strong>
                      {part.label && <span>{part.label}</span>}
                    </div>
                    <div className="storage-partition-details">
                      <span>{bytes(part.size_bytes)}</span>
                      <span>{part.filesystem || t("unknownFilesystem")}</span>
                      {part.uuid && <span className="mono">UUID {part.uuid}</span>}
                    </div>
                    <div className="storage-mount">
                      <span className={mounted ? "status-badge status-enabled" : "status-badge"}>
                        {mounted ? t("mounted") : t("notMounted")}
                      </span>
                      {mounted && <span className="mono">{part.mountpoints.join(", ")}</span>}
                    </div>
                  </div>

                  <div className="storage-actions">
                    {mounted ? (
                      <button
                        type="button"
                        className="button secondary"
                        disabled={operationBusy || root}
                        onClick={() => perform("unmount", part)}
                      >
                        {operationBusy ? t("working") : t("unmount")}
                      </button>
                    ) : (
                      <button
                        type="button"
                        className="button secondary"
                        disabled={operationBusy}
                        onClick={() => perform("mount", part)}
                      >
                        {operationBusy ? t("working") : t("mount")}
                      </button>
                    )}
                    <button
                      type="button"
                      className="button danger"
                      disabled={operationBusy || mounted || disk.system}
                      onClick={() => {
                        setFormatDevice(formatting ? "" : part.path);
                        setError("");
                      }}
                    >
                      {t("format")}
                    </button>
                  </div>

                  {formatting && (
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
                      <div className="storage-format-warning">
                        {t("formatWarning")}
                      </div>
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
                          onClick={() => format(part)}
                        >
                          {operationBusy ? t("working") : t("format")}
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>

          {disk.system && (
            <div className="storage-protection">{t("systemDiskProtection")}</div>
          )}
        </section>
      ))}
    </div>
  );
}
