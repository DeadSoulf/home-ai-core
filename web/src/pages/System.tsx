import { useCallback, useEffect, useState } from "react";
import { APIError, api } from "../api/client";
import type { UpdateStatus, UpdaterState } from "../api/types";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";
import { StorageDevices } from "../components/StorageDevices";
import { NetworkManagement } from "../components/NetworkManagement";

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
}: {
  revision: number;
  canReadNetwork: boolean;
  canManageNetwork: boolean;
}) {
  const {t, date} = useI18n();
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
    api.updaterState().then(setUpdaterState).catch(() => undefined);
  }, []);

  async function checkUpdate() {
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
    if (!downloadingUpdate) return;
    let stopped = false;
    const poll = async () => {
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
      if (!stopped) window.setTimeout(poll, 750);
    };
    poll();
    return () => {
      stopped = true;
    };
  }, [downloadingUpdate]);

  async function downloadUpdate() {
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
        if (system.version === version) {
          window.location.reload();
          return;
        }
      } catch {
        // Core is expected to be briefly unavailable during replacement.
      }
      if (Date.now() < deadline) {
        window.setTimeout(poll, 1500);
      } else {
        setInstallingUpdate(false);
        setRollingBackUpdate(false);
        onTimeout?.();
        setUpdateError(t("requestFailed"));
      }
    };
    window.setTimeout(poll, 1000);
  }

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;
  const value = data!;

  return (
    <div className="page">
      <PageHeading title={t("system")} subtitle={t("systemSubtitle")} />
      <div className="two-column">
        <Panel title={t("node")}>
          <dl className="details">
            <dt>{t("hostname")}</dt><dd>{value.system.hostname}</dd>
            <dt>{t("nodeId")}</dt><dd className="mono">{value.system.node_id}</dd>
            <dt>{t("os")}</dt><dd>{value.system.os}</dd>
            <dt>{t("kernel")}</dt><dd>{value.system.kernel || "—"}</dd>
            <dt>{t("architecture")}</dt><dd>{value.system.architecture}</dd>
            <dt>{t("coreVersion")}</dt><dd>{value.version}</dd>
          </dl>
        </Panel>

        <Panel title={t("compute")}>
          <dl className="details">
            <dt>CPU</dt><dd>{value.system.cpu.model || t("unknown")}</dd>
            <dt>{t("logicalCpus")}</dt><dd>{value.system.cpu.logical_cpus}</dd>
            <dt>CPU load</dt><dd>{value.system.cpu.usage_percent.toFixed(1)}%</dd>
            <dt>{t("ram")}</dt><dd>{bytes(value.system.memory.total_bytes)}</dd>
            <dt>{t("availableRam")}</dt><dd>{bytes(value.system.memory.available_bytes)}</dd>
            <dt>{t("gpuCount")}</dt><dd>{value.system.gpus.length}</dd>
            <dt>{t("gpu")}</dt><dd>{value.system.gpus.length ? value.system.gpus.map((gpu) => gpu.model || gpu.vendor || gpu.device_id || t("unknown")).join(", ") : "—"}</dd>
          </dl>
        </Panel>

        <Panel
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
            <dt>{t("architecture")}</dt><dd>{updateInfo?.architecture || value.system.architecture}</dd>
            <dt>{t("systemHelper")}</dt><dd className="mono">{updateInfo?.helper_version || "—"}</dd>
            <dt>{t("helperProtocol")}</dt><dd className="mono">{updateInfo?.helper_protocol || "—"}</dd>
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
          {updateInfo?.helper_compatible && (
            <div className="notice">{t("helperReady")}: {updateInfo.helper_version || "—"} · protocol {updateInfo.helper_protocol || "—"}</div>
          )}
          {updaterState &&
            !["idle", "succeeded", "available", "checking"].includes(updaterState.phase) && (
            <div className="notice">
              <div><strong>{updaterState.message || updaterState.phase}</strong></div>
              <div className="progress"><span style={{width: `${Math.min(100, updaterState.progress_percent || 0)}%`}} /></div>
              <div className="small">{updaterState.progress_percent || 0}% · {updaterState.phase}</div>
            </div>
          )}
          {updateInfo?.available && updaterState?.phase !== "ready" && (
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
          {updaterState?.phase === "ready" && (
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
          {updateInfo?.rollback_available && updateInfo.rollback_version && (
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
          {updateError && <div className="form-error">{updateError}</div>}
          {lastChecked && <div className="notice">{t("lastChecked")}: {date(lastChecked)}</div>}
          {updateInfo?.notes && <pre className="release-notes">{updateInfo.notes}</pre>}
        </Panel>

        <Panel title={t("gpuDevices")} className="wide">
          <div className="table-wrap">
            <table>
              <thead><tr><th>{t("model")}</th><th>{t("vendor")}</th><th>Load</th><th>{t("pciAddress")}</th><th>{t("deviceId")}</th><th>{t("driver")}</th></tr></thead>
              <tbody>
                {value.system.gpus.map((gpu) => (
                  <tr key={gpu.pci_address || gpu.card || gpu.device_id}>
                    <td>{gpu.model || t("unknown")}</td>
                    <td>{gpu.vendor || "—"}</td>
                    <td>{gpu.utilization_percent === undefined ? "—" : gpu.utilization_percent.toFixed(1) + "%"}</td>
                    <td className="mono">{gpu.pci_address || "—"}</td>
                    <td className="mono">{gpu.vendor_id && gpu.device_id ? `${gpu.vendor_id}:${gpu.device_id}` : (gpu.device_id || "—")}</td>
                    <td>{gpu.driver || <span className="status-badge status-failed">{t("driverMissing")}</span>}</td>
                  </tr>
                ))}
                {value.system.gpus.length === 0 && <tr><td colSpan={6} className="muted">—</td></tr>}
              </tbody>
            </table>
          </div>
        </Panel>

        <Panel title={t("blockDevices")} className="wide">
          <StorageDevices
            devices={value.system.block_tree}
            onChanged={() => setMetricsTick((current) => current + 1)}
          />
        </Panel>

        <NetworkManagement
          interfaces={value.system.network_interfaces}
          canReadNetwork={canReadNetwork}
          canManage={canManageNetwork}
          onChanged={() => setMetricsTick((current) => current + 1)}
        />
      </div>

    </div>
  );
}
