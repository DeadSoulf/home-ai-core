import { useCallback, useEffect, useRef, useState } from "react";
import { APIError, api } from "../api/client";
import type { UpdateStatus, UpdaterState } from "../api/types";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";
import { StorageBrowser } from "../components/StorageBrowser";
import { NetworkManagement } from "../components/NetworkManagement";
import { ServerConsole } from "../components/ServerConsole";
import { UpdateOverview } from "../components/UpdateOverview";
import { useUI } from "../ui/useUI";
import type { SystemSection } from "../navigation";

function bytes(value = 0) {
  return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
}

export function SystemPage({
  revision,
  canReadNetwork,
  canManageNetwork,
  section = "equipment",
  onSectionChange,
  canReadUpdates,
  canManageUpdates,
  canManageStorage,
  canUseConsole,
}: {
  revision: number;
  canReadNetwork: boolean;
  canManageNetwork: boolean;
  section?: SystemSection;
  onSectionChange: (section: SystemSection) => void;
  canReadUpdates: boolean;
  canManageUpdates: boolean;
  canManageStorage: boolean;
  canUseConsole: boolean;
}) {
  const {t, date} = useI18n();
  const u = useUI();
  const activeSection =
    (section === "updates" && !canReadUpdates) ||
    (section === "console" && !canUseConsole)
      ? "equipment"
      : section;
  const mounted = useRef(true);
  const timers = useRef(new Set<number>());
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      for (const timer of timers.current) window.clearTimeout(timer);
      timers.current.clear();
    };
  }, []);
  function schedule(callback: () => void, delay: number) {
    const timer = window.setTimeout(() => {
      timers.current.delete(timer);
      if (mounted.current) callback();
    }, delay);
    timers.current.add(timer);
  }
  const load = useCallback((signal: AbortSignal) => api.system(signal), []);
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<UpdateStatus>();
  const [updaterState, setUpdaterState] = useState<UpdaterState>();
  const [downloadingUpdate, setDownloadingUpdate] = useState(false);
  const [installingUpdate, setInstallingUpdate] = useState(false);
  const [rollingBackUpdate, setRollingBackUpdate] = useState(false);
  const [updateError, setUpdateError] = useState("");
  const [lastChecked, setLastChecked] = useState<string>();
  const {data, loading, slow, error, updatedAt, reload} = useResource(load, revision, 2000);

  useEffect(() => {
    if (!canReadUpdates) return;
    let stopped = false;
    api.updaterState().then((state) => { if (!stopped) setUpdaterState(state); }).catch(() => undefined);
    return () => { stopped = true; };
  }, [canReadUpdates]);

  async function checkUpdate() {
    if (!canReadUpdates) return;
    setCheckingUpdate(true);
    setUpdateError("");
    try {
      const result = await api.updateStatus(true);
      setUpdateInfo(result);
      window.dispatchEvent(new CustomEvent<string | undefined>(
        "home-ai-core:update-status",
        {detail: result.available ? result.available_version : undefined},
      ));
      setUpdaterState(await api.updaterState());
      setLastChecked(new Date().toISOString());
    } catch (reason) {
      if (reason instanceof APIError && reason.code === "update_check_failed") {
        setUpdateError(t("updateCheckUnavailable"));
      } else {
        setUpdateError(reason instanceof Error ? reason.message : t("requestFailed"));
      }
    } finally {
      setCheckingUpdate(false);
    }
  }

  useEffect(() => {
    if (activeSection !== "updates" || !canReadUpdates) return;
    void checkUpdate();
  }, [activeSection, canReadUpdates]);

  useEffect(() => {
    if (!canReadUpdates || !downloadingUpdate) return;
    let stopped = false;
    const poll = async () => {
      if (stopped || !mounted.current) return;
      try {
        const state = await api.updaterState();
        if (stopped) return;
        setUpdaterState(state);
        if (state.phase === "ready" || state.phase === "failed") {
          setDownloadingUpdate(false);
          if (state.error) setUpdateError(state.error);
          return;
        }
      } catch {
        // Keep polling while the download request is in flight.
      }
      if (!stopped) schedule(poll, 750);
    };
    poll();
    return () => {
      stopped = true;
    };
  }, [canReadUpdates, downloadingUpdate]);

  async function downloadUpdate() {
    if (!canManageUpdates) return;
    const version = updateInfo?.available_version;
    if (!version) return;
    setDownloadingUpdate(true);
    setUpdateError("");
    try {
      const state = await api.downloadUpdate(version);
      setUpdaterState(state);
    } catch (reason) {
      setUpdateError(reason instanceof Error ? reason.message : t("requestFailed"));
      try {
        setUpdaterState(await api.updaterState());
      } catch {
        // Keep the original download error.
      }
    } finally {
      setDownloadingUpdate(false);
    }
  }

  async function installUpdate() {
    if (!canManageUpdates) return;
    const version = updaterState?.available_version || updateInfo?.available_version;
    if (!version) return;

    setInstallingUpdate(true);
    setUpdateError("");
    try {
      const state = await api.installUpdate(version);
      setUpdaterState(state);
    } catch (reason) {
      if (reason instanceof APIError) {
        setUpdateError(reason.code === "helper_upgrade_required" ? t("helperUpgradeRequired") : reason.message);
        setInstallingUpdate(false);
        return;
      }
      // Core may stop immediately after the privileged helper accepts the update.
    }
    waitForUpdatedVersion(version);
  }

  async function rollbackUpdate() {
    if (!canManageUpdates) return;
    const version = updateInfo?.rollback_version;
    if (!version) return;

    setRollingBackUpdate(true);
    setUpdateError("");
    try {
      const state = await api.rollbackUpdate();
      setUpdaterState(state);
    } catch (reason) {
      if (reason instanceof APIError) {
        setUpdateError(reason.code === "helper_upgrade_required" ? t("helperUpgradeRequired") : reason.message);
        setRollingBackUpdate(false);
        return;
      }
      // Core may stop immediately after the privileged helper accepts rollback.
    }
    waitForUpdatedVersion(version, () => setRollingBackUpdate(false));
  }

  function waitForUpdatedVersion(version: string, onTimeout?: () => void) {
    const deadline = Date.now() + 3 * 60 * 1000;
    const poll = async () => {
      try {
        const system = await api.system();
        if (!mounted.current) return;
        if (system.version === version) {
          window.location.reload();
          return;
        }
      } catch {
        // Core is expected to be briefly unavailable during replacement.
      }
      if (!mounted.current) return;
      if (Date.now() < deadline) {
        schedule(poll, 1500);
      } else {
        setInstallingUpdate(false);
        setRollingBackUpdate(false);
        onTimeout?.();
        setUpdateError(t("requestFailed"));
      }
    };
    schedule(poll, 1000);
  }

  if (loading && !data) return <><LoadingState />{slow && <p role="status">{u("slow")}</p>}</>;
  if (error && !data) return <ErrorState message={error} onRetry={reload} />;
  const value = data!;

  return (
    <div className="page">
      <PageHeading title={t("system")} subtitle={t("serverOverview")} />
      <div className="resource-summary">
        <span>{u("lastUpdated")}: {updatedAt ? date(new Date(updatedAt).toISOString()) : "—"}</span>
        {slow && <span role="status">{u("slow")}</span>}
        <button className="button secondary" type="button" disabled={loading} onClick={reload}>{u("refresh")}</button>
      </div>
      <nav className="server-sections" aria-label={t("serverSections")}>
        {(["equipment", "storage", "network", "updates", "console"] as const)
          .filter((item) =>
            (item !== "updates" || canReadUpdates) &&
            (item !== "console" || canUseConsole)
          )
          .map((item) => <button type="button" key={item} className={`button secondary${activeSection === item ? " active" : ""}`}
            aria-current={activeSection === item ? "page" : undefined} onClick={() => onSectionChange(item)}>{t(item)}</button>)}
      </nav>
      {error && <ErrorState message={error} />}
      <div className="two-column">
        {activeSection === "equipment" && <>
        <Panel title={t("node")}>
          <dl className="details">
            <dt>{t("hostname")}</dt><dd>{value.system.hostname}</dd>
            <dt>{t("os")}</dt><dd>{value.system.os}</dd>
            <dt>{t("coreVersion")}</dt><dd>{value.version}</dd>
          </dl>
          <details className="technical-details"><summary>{t("hostDetails")}</summary>
            <dl className="details">
              <dt>{t("nodeId")}</dt><dd className="mono">{value.system.node_id}</dd>
              <dt>{t("kernel")}</dt><dd>{value.system.kernel || "—"}</dd>
              <dt>{t("architecture")}</dt><dd>{value.system.architecture}</dd>
            </dl>
          </details>
        </Panel>

        <Panel title={t("compute")}>
          <dl className="details">
            <dt>CPU</dt><dd>{value.system.cpu.model || t("unknown")}</dd>
            <dt>{t("logicalCpus")}</dt><dd>{value.system.cpu.logical_cpus}</dd>
            <dt>{t("cpuLoad")}</dt><dd>{value.system.cpu.usage_percent.toFixed(1)}%</dd>
            <dt>{t("ram")}</dt><dd>{bytes(value.system.memory.total_bytes)}</dd>
            <dt>{t("availableRam")}</dt><dd>{bytes(value.system.memory.available_bytes)}</dd>
          </dl>
        </Panel>

        </>}
        {activeSection === "updates" && canReadUpdates &&
          <UpdateOverview
            installedVersion={value.version}
            architecture={value.system.architecture}
            canManage={canManageUpdates}
            status={updateInfo}
            state={updaterState}
            checking={checkingUpdate}
            downloading={downloadingUpdate}
            installing={installingUpdate}
            rollingBack={rollingBackUpdate}
            error={updateError}
            lastChecked={lastChecked}
            onCheck={() => {void checkUpdate();}}
            onDownload={() => {void downloadUpdate();}}
            onInstall={() => {void installUpdate();}}
            onRollback={() => {void rollbackUpdate();}}
          />}

        {activeSection === "equipment" && <Panel title={t("gpuDevices")} className="wide">
          <div className="table-wrap">
            <table>
              <thead><tr><th>{t("model")}</th><th>{t("gpuLoad")}</th><th>{t("driver")}</th><th>{t("hardwareDetails")}</th></tr></thead>
              <tbody>
                {value.system.gpus.map((gpu) => (
                  <tr key={gpu.pci_address || gpu.card || gpu.device_id}>
                    <td>{gpu.model || t("unknown")}</td>
                    <td>{gpu.utilization_percent === undefined ? "—" : gpu.utilization_percent.toFixed(1) + "%"}</td>
                    <td><span className={`status-badge ${gpu.driver ? "status-success" : "status-failed"}`}>{gpu.driver ? t("driverDetected") : t("driverMissing")}</span></td>
                    <td><details className="technical-details row-details"><summary>{t("technicalDetails")}</summary>
                      <dl className="details">
                        <dt>{t("vendor")}</dt><dd>{gpu.vendor || "—"}</dd>
                        <dt>{t("driver")}</dt><dd>{gpu.driver || "—"}</dd>
                        <dt>{t("pciAddress")}</dt><dd className="mono">{gpu.pci_address || "—"}</dd>
                        <dt>{t("deviceId")}</dt><dd className="mono">{gpu.vendor_id && gpu.device_id ? `${gpu.vendor_id}:${gpu.device_id}` : gpu.device_id || "—"}</dd>
                      </dl>
                    </details></td>
                  </tr>
                ))}
                {value.system.gpus.length === 0 && <tr><td colSpan={4} className="muted">{t("noGpuDevices")}</td></tr>}
              </tbody>
            </table>
          </div>
        </Panel>

        }
        {activeSection === "storage" && <Panel title={t("blockDevices")} className="wide">
          <StorageBrowser
            devices={value.system.block_tree}
            canManage={canManageStorage}
            onChanged={reload}
          />
        </Panel>

        }
        {activeSection === "console" && canUseConsole && <ServerConsole />}

        {activeSection === "network" && <NetworkManagement
          interfaces={value.system.network_interfaces}
          canReadNetwork={canReadNetwork}
          canManage={canManageNetwork}
          onChanged={reload}
        />}
      </div>

    </div>
  );
}
