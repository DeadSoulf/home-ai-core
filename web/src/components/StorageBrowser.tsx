import { useEffect, useMemo, useState } from "react";
import type { BlockNode } from "../api/types";
import { useI18n } from "../i18n";
import { StorageDevices } from "./StorageDevices";
import "./StorageBrowser.css";

function bytes(value = 0) {
  if (value >= 1024 ** 4) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 4) + " TiB";
  if (value >= 1024 ** 3) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
  if (value >= 1024 ** 2) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 2) + " MiB";
  if (value >= 1024) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024) + " KiB";
  return value + " B";
}

function diskKey(node: BlockNode): string {
  return node.path || node.name;
}

function descendantCount(node: BlockNode): number {
  return (node.children || []).reduce((total, child) => total + 1 + descendantCount(child), 0);
}

function mountedDescendants(node: BlockNode): number {
  return (node.children || []).reduce(
    (total, child) =>
      total +
      (child.mountpoints.length > 0 ? 1 : 0) +
      mountedDescendants(child),
    0,
  );
}

function healthClass(health?: string): string {
  if (health === "ok") return "status-badge status-success";
  if (health === "warning") return "status-badge status-queued";
  if (health === "failed") return "status-badge status-failed";
  return "status-badge";
}

function healthText(node: BlockNode, t: ReturnType<typeof useI18n>["t"]): string {
  if (node.health === "ok") return t("diskHealthOk");
  if (node.health === "warning") return t("diskHealthWarning");
  if (node.health === "failed") return t("diskHealthFailed");
  if (!node.smart_available) return t("smartUnavailable");
  return t("unknown");
}

export function StorageBrowser({
  devices,
  onChanged,
  canManage = false,
}: {
  devices: BlockNode[];
  onChanged: () => void;
  canManage?: boolean;
}) {
  const {t} = useI18n();
  const disks = useMemo(() => devices.filter((node) => node.type === "disk"), [devices]);
  const [selectedKey, setSelectedKey] = useState("");

  const selectedDisk = useMemo(
    () => disks.find((disk) => diskKey(disk) === selectedKey),
    [disks, selectedKey],
  );

  useEffect(() => {
    if (selectedKey && !selectedDisk) setSelectedKey("");
  }, [selectedKey, selectedDisk]);

  if (disks.length === 0) {
    return <div className="empty-state">{t("noBlockDevices")}</div>;
  }

  if (!selectedDisk) {
    return (
      <div className="storage-browser">
        <div className="storage-browser-list">
          {disks.map((disk) => {
            const children = descendantCount(disk);
            const mounted = mountedDescendants(disk);
            const model = [disk.vendor, disk.model].filter(Boolean).join(" ");
            return (
              <button
                type="button"
                className="storage-disk-row"
                key={diskKey(disk)}
                onClick={() => setSelectedKey(diskKey(disk))}
              >
                <div className="storage-disk-identity">
                  <span className="storage-disk-icon" aria-hidden="true">▰</span>
                  <span className="storage-disk-copy">
                    <span className="storage-disk-title">
                      <strong>{disk.display_name || disk.name}</strong>
                      {disk.system && <span className="status-badge storage-system">{t("systemDisk")}</span>}
                      <span className={healthClass(disk.health)}>{healthText(disk, t)}</span>
                    </span>
                    <span className="storage-disk-meta">
                      {model || disk.path || disk.name}
                      {disk.transport ? " · " + disk.transport.toUpperCase() : ""}
                      {disk.path ? " · " + disk.path : ""}
                    </span>
                  </span>
                </div>

                <span className="storage-disk-stat">
                  <span>{t("size")}</span>
                  <strong>{bytes(disk.size_bytes)}</strong>
                </span>

                <span className="storage-disk-stat">
                  <span>{t("partitionTable")}</span>
                  <strong>{(disk.partition_table || "—").toUpperCase().replace("DOS", "MBR")}</strong>
                  <small>{children} · {mounted} {t("mountPoints").toLowerCase()}</small>
                </span>

                <span className="storage-disk-stat">
                  <span>{t("unallocated")}</span>
                  <strong>{bytes(disk.unallocated_bytes || 0)}</strong>
                </span>

                <span className="storage-disk-chevron" aria-hidden="true">›</span>
              </button>
            );
          })}
        </div>

        {disks.some((disk) => disk.system) && (
          <div className="storage-protection storage-browser-protection">{t("systemDiskProtection")}</div>
        )}
      </div>
    );
  }

  const model = [selectedDisk.vendor, selectedDisk.model].filter(Boolean).join(" ");
  const title = selectedDisk.display_name || selectedDisk.name;

  return (
    <div className="storage-disk-page">
      <div className="storage-disk-page-header">
        <button
          type="button"
          className="button secondary compact"
          onClick={() => setSelectedKey("")}
        >
          ← {t("blockDevices")}
        </button>

        <div className="storage-disk-page-title">
          <div>
            <strong>{title}</strong>
            <span className="mono">{selectedDisk.path || selectedDisk.name}</span>
          </div>
          <div className="storage-disk-page-badges">
            {selectedDisk.system && <span className="status-badge storage-system">{t("systemDisk")}</span>}
            <span className={healthClass(selectedDisk.health)}>{healthText(selectedDisk, t)}</span>
          </div>
        </div>
      </div>

      <div className="storage-disk-summary">
        <dl className="details">
          <dt>{t("model")}</dt><dd>{model || "—"}</dd>
          <dt>{t("size")}</dt><dd>{bytes(selectedDisk.size_bytes)}</dd>
          <dt>{t("partitionTable")}</dt><dd>{(selectedDisk.partition_table || "—").toUpperCase().replace("DOS", "MBR")}</dd>
          <dt>{t("unallocated")}</dt><dd>{bytes(selectedDisk.unallocated_bytes || 0)}</dd>
          <dt>{t("type")}</dt><dd>{selectedDisk.transport?.toUpperCase() || t("diskTypeDisk")}</dd>
          <dt>{t("state")}</dt><dd>{healthText(selectedDisk, t)}</dd>
        </dl>
        <div className="storage-disk-health">
          {selectedDisk.temperature_c !== undefined && (
            <span><small>°C</small><strong>{selectedDisk.temperature_c}</strong></span>
          )}
          {selectedDisk.power_on_hours !== undefined && (
            <span><small>{t("hoursShort")}</small><strong>{selectedDisk.power_on_hours.toLocaleString()}</strong></span>
          )}
          {selectedDisk.life_remaining_percent !== undefined && (
            <span><small>{t("lifeRemaining")}</small><strong>{selectedDisk.life_remaining_percent}%</strong></span>
          )}
        </div>
        <div className="storage-disk-summary-actions">
          <StorageDevices
            devices={[selectedDisk]}
            canManage={canManage}
            onChanged={onChanged}
            mode="diskActions"
          />
        </div>
      </div>

      <div className="storage-disk-manager">
        <StorageDevices
          devices={[selectedDisk]}
          canManage={canManage}
          onChanged={onChanged}
          mode="partitions"
          expandedByDefault
          showDiskToolbar={false}
        />
      </div>
    </div>
  );
}
