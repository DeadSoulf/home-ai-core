import { useState } from "react";
import { api } from "../api/client";
import type { BlockNode } from "../api/types";
import { useI18n } from "../i18n";

const minBlankDiskBytes = 16 * 1024 * 1024;
type DiskSetup = {kind: "blank"} | {kind: "existing"; partition: BlockNode} | {kind: "unavailable"};

export function quickDiskSetup(disk: BlockNode): DiskSetup {
  if (disk.type !== "disk" || !disk.path || disk.system || disk.health === "failed" ||
      disk.mountpoints.length > 0) return {kind: "unavailable"};
  const parts = disk.children || [];
  if (parts.length === 0 && !disk.filesystem && !disk.partition_table &&
      (disk.unallocated_bytes || 0) >= minBlankDiskBytes &&
      (disk.size_bytes || 0) >= minBlankDiskBytes) {
    return {kind: "blank"};
  }
  if (parts.length === 1) {
    const part = parts[0];
    if (part.type === "part" && part.path && !part.system &&
        ["ext4", "xfs"].includes((part.filesystem || "").toLowerCase()) &&
        part.mountpoints.length === 0 && (part.children || []).length === 0) {
      return {kind: "existing", partition: part};
    }
  }
  return {kind: "unavailable"};
}

function sameDisk(before: BlockNode, after: BlockNode): boolean {
  return before.path === after.path && before.type === after.type &&
    before.size_bytes === after.size_bytes &&
    before.serial === after.serial &&
    before.model === after.model;
}

export function QuickDiskSetup({disk, onChanged}: {disk: BlockNode; onChanged: () => void}) {
  const {t} = useI18n();
  const setup = quickDiskSetup(disk);
  const [expanded, setExpanded] = useState(false);
  const [displayName, setDisplayName] = useState(disk.display_name || "");
  const [confirmation, setConfirmation] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  const [busy, setBusy] = useState(false);
  const [phase, setPhase] = useState("");
  const [error, setError] = useState("");
  const [result, setResult] = useState("");

  if (setup.kind === "unavailable") return null;

  async function run() {
    if (!disk.path || busy || confirmation.trim() !== disk.path || !acknowledged) return;
    let modified = false;
    setBusy(true);
    setError("");
    setResult("");
    try {
      setPhase(t("quickDiskChecking"));
      // A fresh hardware inventory is required so a replaced /dev path cannot
      // silently direct this wizard at an unrelated disk.
      const current = await api.system();
      const liveDisk = current.system.block_tree.find((item) => item.path === disk.path);
      if (!liveDisk || !sameDisk(disk, liveDisk)) {
        throw new Error(t("quickDiskChanged"));
      }
      const liveSetup = quickDiskSetup(liveDisk);
      if (liveSetup.kind !== setup.kind ||
          (setup.kind === "existing" && (liveSetup.kind !== "existing" ||
            liveSetup.partition.path !== setup.partition.path ||
            liveSetup.partition.uuid !== setup.partition.uuid))) {
        throw new Error(t("quickDiskChanged"));
      }

      let partition: string;
      if (setup.kind === "blank") {
        await api.storageOperation({operation: "preflight", device: disk.path});
        await api.storageOperation({
          operation: "partition.create", device: disk.path,
          purpose: "files", dry_run: true,
        });
        setPhase(t("quickDiskCreating"));
        const created = await api.storageOperation({
          operation: "partition.create", device: disk.path,
          purpose: "files", confirm: `CREATE ${disk.path}`,
        });
        modified = true;
        if (!created.device) {
          throw new Error(t("quickDiskPartitionUnknown"));
        }
        // Only the newly reported partition may be formatted. Do not guess
        // /dev/sdX1 or use an old partition selected before creating it.
        const refreshed = await api.system();
        const newDisk = refreshed.system.block_tree.find((item) => item.path === disk.path);
        const newPart = newDisk?.children.find((item) => item.path === created.device);
        if (!newDisk || newDisk.system || !newPart || newPart.type !== "part" ||
            newPart.system || newPart.filesystem || newPart.mountpoints.length > 0) {
          throw new Error(t("quickDiskPartitionUnknown"));
        }
        partition = created.device;
        await api.storageOperation({
          operation: "format", device: partition,
          filesystem: "ext4", dry_run: true,
        });
        setPhase(t("quickDiskFormatting"));
        await api.storageOperation({
          operation: "format", device: partition,
          filesystem: "ext4", confirm: `FORMAT ${partition}`,
        });
      } else {
        partition = setup.partition.path!;
      }

      setPhase(t("quickDiskMounting"));
      await api.storageOperation({operation: "mount", device: partition, dry_run: true});
      await api.storageOperation({operation: "mount", device: partition});
      modified = true;
      setPhase(t("quickDiskAssigning"));
      await api.setStoragePurpose(partition, "files");
      if (displayName.trim()) {
        await api.setDiskName(disk.path, displayName.trim());
      }
      onChanged();
      setPhase("");
      setResult(t("quickDiskDone"));
      setExpanded(false);
    } catch (reason) {
      setPhase("");
      setError((reason instanceof Error ? reason.message : t("requestFailed")) +
        (modified ? "\n" + t("quickDiskPartial") : ""));
      if (modified) onChanged();
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="quick-disk-setup">
      <div className="quick-disk-setup-heading">
        <div>
          <strong>{t(setup.kind === "blank" ? "quickDiskBlankTitle" : "quickDiskExistingTitle")}</strong>
          <p>{t(setup.kind === "blank" ? "quickDiskBlankDescription" : "quickDiskExistingDescription")}</p>
        </div>
        {!expanded && !result && (
          <button type="button" className="button primary" onClick={() => setExpanded(true)}>
            {t("quickDiskStart")}
          </button>
        )}
      </div>
      {result && <div className="storage-success" role="status">{result}</div>}
      {error && <div className="form-error" role="alert">{error}</div>}
      {expanded && (
        <div className="quick-disk-setup-form">
          <div className="quick-disk-setup-target">
            <strong>{disk.display_name || disk.model || disk.name}</strong>
            <span className="mono">{disk.path} · {(disk.size_bytes || 0) / 1024 ** 3 >= 1
              ? `${((disk.size_bytes || 0) / 1024 ** 3).toFixed(1)} GiB`
              : `${((disk.size_bytes || 0) / 1024 ** 2).toFixed(0)} MiB`}</span>
          </div>
          <label>
            {t("diskDisplayName")} ({t("optional")})
            <input value={displayName} maxLength={64} disabled={busy}
              onChange={(event) => setDisplayName(event.target.value)}
              placeholder={t("diskDisplayNamePlaceholder")} />
          </label>
          <div className="quick-disk-setup-plan">
            {setup.kind === "blank" ? t("quickDiskPlanBlank") : t("quickDiskPlanExisting")}
          </div>
          <div className="storage-format-warning">
            {t(setup.kind === "blank" ? "quickDiskWarningBlank" : "quickDiskWarningExisting")}
          </div>
          <label className="quick-disk-setup-ack">
            <input type="checkbox" checked={acknowledged} disabled={busy}
              onChange={(event) => setAcknowledged(event.target.checked)} />
            {t("quickDiskAcknowledge")}
          </label>
          <label>
            {t("quickDiskTypeDevice").replace("{device}", disk.path)}
            <input value={confirmation} disabled={busy} className="mono"
              onChange={(event) => setConfirmation(event.target.value)}
              autoComplete="off" spellCheck={false} />
          </label>
          {phase && <div className="storage-operation-progress" role="status" aria-live="polite">
            <strong>{phase}</strong>
            <div className="storage-operation-progress-track">
              <div className="storage-operation-progress-bar" />
            </div>
          </div>}
          <div className="storage-format-actions">
            <button type="button" className="button secondary" disabled={busy}
              onClick={() => setExpanded(false)}>{t("cancel")}</button>
            <button type="button" className="button primary"
              disabled={busy || !acknowledged || confirmation.trim() !== disk.path}
              onClick={() => void run()}>
              {busy ? t("working") : t("quickDiskConfirm")}
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
