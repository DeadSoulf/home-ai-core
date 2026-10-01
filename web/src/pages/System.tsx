import { useCallback, useEffect, useRef, useState } from "react";
import { APIError, api } from "../api/client";
import type { UpdateStatus, UpdaterState } from "../api/types";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";
import { StorageBrowser } from "../components/StorageBrowser";
import { NetworkManagement } from "../components/NetworkManagement";
import type { SystemSection } from "../navigation";

function bytes(value = 0) {
  return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
}

function fileBytes(value = 0) {
  if (value >= 1024 ** 3) return bytes(value);
  if (value >= 1024 ** 2) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 2) + " MiB";
  if (value >= 1024) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024) + " KiB";
  return value + " B";
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
}: {
  revision: number;
  canReadNetwork: boolean;
  canManageNetwork: boolean;
  section?: SystemSection;
  onSectionChange: (section: SystemSection) => void;
  canReadUpdates: boolean;
  canManageUpdates: boolean;
  canManageStorage: boolean;
}) {
  const {t, date} = useI18n();
  const activeSection = section === "updates" && !canReadUpdates ? "equipment" : section;
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
  const load = useCallback(() => api.system(), []);
  const [metricsTick, setMetricsTick] = useState(0);
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<UpdateStatus>();
  const [updaterState, setUpdaterState] = useState<UpdaterState>();
  const [downloadingUpdate, setDownloadingUpdate] = useState(false);
  const [installingUpdate, setInstallingUpdate] = useState(false);
  const [rollingBackUpdate, setRollingBackUpdate] = useState(false);
  const [updateError, setUpdateError] = useState("");
  const [lastChecked, setLastChecked] = useState<string>();
  const {data, loading, error} = useResource(load, revision + metricsTick);

  useEffect(() => {
    const timer = window.setInterval(() => setMetricsTick((value) => value + 1), 2000);
    return () => window.clearInterval(timer);
  }, []);

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
    if (!window.confirm(t("rollbackConfirmation").replace("{version}", version))) return;

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

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;
  const value = data!;

  return (
    <div className="page">
      <PageHeading title={t("system")} subtitle={t("serverOverview")} />
      <nav className="server-sections" aria-label={t("serverSections")}>
        {(["equipment", "storage", "network", "updates"] as const)
          .filter((item) => item !== "updates" || canReadUpdates)
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
        {activeSection === "updates" && canReadUpdates && <Panel
          className="wide"
          title={t("updateStatus")}
          action={
            <button
              type="button"
              className="button secondary"
              disabled={checkingUpdate}
              onClick={checkUpdate}
            >
              {checkingUpdate ? t("checking") : t("checkUpdates")}
            </button>
          }
        >
          <dl className="details">
            <dt>{t("currentVersion")}</dt><dd className="mono">{updateInfo?.current_version || value.version}</dd>
            <dt>{t("availableVersion")}</dt><dd className="mono">{updateInfo?.available_version || "—"}</dd>
            <dt>{t("size")}</dt><dd>{updateInfo?.bundle_size_bytes ? fileBytes(updateInfo.bundle_size_bytes) : "—"}</dd>
            <dt>{t("rollbackVersion")}</dt><dd className="mono">{updateInfo?.rollback_available ? (updateInfo.rollback_version || "—") : t("notAvailable")}</dd>
            {updateInfo?.published_at && <><dt>{t("published")}</dt><dd>{date(updateInfo.published_at)}</dd></>}
          </dl>
          {updateInfo && (
            <div className={updateInfo.available ? "update-callout available" : "update-callout current"}>
              <strong>{updateInfo.available ? t("updateAvailable") : t("upToDate")}</strong>
            </div>
          )}
          {updateInfo && !updateInfo.helper_available && (
            <div className="form-error">{t("helperUnavailable")}</div>
          )}
          {updateInfo?.helper_available && !updateInfo.helper_compatible && (
            <div className="form-error">{t("helperUpgradeRequired")}</div>
          )}
          <details className="technical-details"><summary>{t("technicalDetails")}</summary>
            <dl className="details">
              <dt>{t("architecture")}</dt><dd>{updateInfo?.architecture || value.system.architecture}</dd>
              <dt>{t("systemHelper")}</dt><dd>{updateInfo?.helper_version || "—"}</dd>
              <dt>{t("helperProtocol")}</dt><dd>{updateInfo?.helper_protocol || "—"}</dd>
              {updaterState && <><dt>{t("state")}</dt><dd>{updaterState.phase}</dd><dt>{t("progress")}</dt><dd>{updaterState.message || "—"}</dd></>}
            </dl>
          </details>
          {updaterState &&
            ["downloading", "installing", "rolling_back", "restarting"].includes(updaterState.phase) && (
            <div className="notice">
              <div><strong>{updaterState.phase === "downloading" ? t("downloadingUpdate") : updaterState.phase === "installing" ? t("installing") : updaterState.phase === "rolling_back" ? t("rollingBack") : t("working")}</strong></div>
              <div className="progress"><span style={{width: `${Math.min(100, updaterState.progress_percent || 0)}%`}} /></div>
              <div className="small">{updaterState.progress_percent || 0}%</div>
            </div>
          )}
          {canManageUpdates && updateInfo?.available && updaterState?.phase !== "ready" && (
            <div className="update-action-stack">
              <button
                type="button"
                className="button primary"
                disabled={downloadingUpdate || checkingUpdate}
                onClick={downloadUpdate}
              >
                {downloadingUpdate ? t("downloadingUpdate") : t("downloadUpdate")}
              </button>
            </div>
          )}
          {canManageUpdates && updaterState?.phase === "ready" && (
            <div className="update-action-stack">
              <div className="update-callout current"><strong>{t("updateReady")}</strong></div>
              <button
                type="button"
                className="button primary"
                disabled={installingUpdate || rollingBackUpdate || updateInfo?.helper_available === false || updateInfo?.helper_compatible === false}
                onClick={installUpdate}
              >
                {installingUpdate ? t("installing") : t("installUpdate")}
              </button>
              <div className="notice">{t("updateRestartNotice")}</div>
            </div>
          )}
          {canManageUpdates && updateInfo?.rollback_available && updateInfo.rollback_version && (
            <div className="update-action-stack">
              <button
                type="button"
                className="button danger"
                disabled={installingUpdate || rollingBackUpdate || updateInfo.helper_available === false || updateInfo.helper_compatible === false}
                onClick={rollbackUpdate}
              >
                {rollingBackUpdate ? t("rollingBack") : t("rollbackUpdate").replace("{version}", updateInfo.rollback_version)}
              </button>
              <div className="notice">{t("rollbackNotice")}</div>
            </div>
          )}
          {(updateError || updaterState?.phase === "failed") && <div className="form-error">{updateError || updaterState?.error || t("requestFailed")}</div>}
          {lastChecked && <div className="notice">{t("lastChecked")}: {date(lastChecked)}</div>}
          {updateInfo?.notes && <details className="technical-details"><summary>{t("releaseNotes")}</summary><pre className="release-notes">{updateInfo.notes}</pre></details>}
        </Panel>}

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
            onChanged={() => setMetricsTick((current) => current + 1)}
          />
        </Panel>

        }
        {activeSection === "network" && <NetworkManagement
          interfaces={value.system.network_interfaces}
          canReadNetwork={canReadNetwork}
          canManage={canManageNetwork}
          onChanged={() => setMetricsTick((current) => current + 1)}
        />}
      </div>

    </div>
  );
}
