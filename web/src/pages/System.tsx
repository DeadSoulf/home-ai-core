import { useCallback, useEffect, useState } from "react";
import { APIError, api } from "../api/client";
import type { UpdateStatus, UpdaterState } from "../api/types";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";

function bytes(value = 0) {
  return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
}

function fileBytes(value = 0) {
  if (value >= 1024 ** 3) return bytes(value);
  if (value >= 1024 ** 2) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 2) + " MiB";
  if (value >= 1024) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024) + " KiB";
  return value + " B";
}

export function SystemPage({revision}: {revision: number}) {
  const {t, status, date} = useI18n();
  const load = useCallback(() => api.system(), []);
  const [metricsTick, setMetricsTick] = useState(0);
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<UpdateStatus>();
  const [updaterState, setUpdaterState] = useState<UpdaterState>();
  const [downloadingUpdate, setDownloadingUpdate] = useState(false);
  const [installingUpdate, setInstallingUpdate] = useState(false);
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
      const result = await api.updateStatus();
      setUpdateInfo(result);
      setUpdaterState(await api.updaterState());
      setLastChecked(new Date().toISOString());
    } catch (reason) {
      setUpdateError(reason instanceof Error ? reason.message : t("requestFailed"));
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
        setUpdateError(reason.message);
        setInstallingUpdate(false);
        return;
      }
      // Core may stop immediately after the privileged helper accepts the update.
    }
    waitForUpdatedVersion(version);
  }

  function waitForUpdatedVersion(version: string) {
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
            <dt>{t("size")}</dt><dd>{updateInfo?.bundle_size_bytes ? fileBytes(updateInfo.bundle_size_bytes) : "—"}</dd>
            {updateInfo?.published_at && <><dt>{t("published")}</dt><dd>{date(updateInfo.published_at)}</dd></>}
          </dl>
          {updateInfo && (
            <div className={updateInfo.available ? "update-callout available" : "update-callout current"}>
              <strong>{updateInfo.available ? t("updateAvailable") : t("upToDate")}</strong>
            </div>
          )}
          {updaterState && updaterState.phase !== "idle" && updaterState.phase !== "succeeded" && (
            <div className="notice">
              <div><strong>{updaterState.message || updaterState.phase}</strong></div>
              <div className="progress"><span style={{width: `${Math.min(100, updaterState.progress_percent || 0)}%`}} /></div>
              <div className="small">{updaterState.progress_percent || 0}% · {updaterState.phase}</div>
            </div>
          )}
          {updateInfo?.available && updaterState?.phase !== "ready" && (
            <button
              type="button"
              className="button primary"
              disabled={downloadingUpdate || checkingUpdate}
              onClick={downloadUpdate}
            >
              {downloadingUpdate ? t("downloadingUpdate") : t("downloadUpdate")}
            </button>
          )}
          {updaterState?.phase === "ready" && (
            <>
              <div className="update-callout current"><strong>{t("updateReady")}</strong></div>
              <button
                type="button"
                className="button primary"
                disabled={installingUpdate}
                onClick={installUpdate}
              >
                {installingUpdate ? t("installing") : t("installUpdate")}
              </button>
              <div className="notice">{t("updateRestartNotice")}</div>
            </>
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
          <div className="table-wrap">
            <table>
              <thead><tr><th>{t("device")}</th><th>{t("model")}</th><th>{t("size")}</th><th>{t("type")}</th><th>{t("serial")}</th><th>Filesystem / mount</th></tr></thead>
              <tbody>
                {value.system.block_devices.flatMap((disk) => [
                  <tr key={disk.name}>
                    <td className="mono"><strong>{disk.path}</strong></td>
                    <td>{[disk.vendor, disk.model].filter(Boolean).join(" ") || "—"}</td>
                    <td>{bytes(disk.size_bytes)}</td>
                    <td>{disk.rotational ? "HDD" : t("flash")}</td>
                    <td className="mono">{disk.serial || "—"}</td>
                    <td>{disk.partitions.length ? disk.partitions.length + " partition(s)" : "No partitions"}</td>
                  </tr>,
                  ...disk.partitions.map((part) => (
                    <tr key={part.name}>
                      <td className="mono">↳ {part.path}</td>
                      <td className="muted">Partition</td>
                      <td>{bytes(part.size_bytes)}</td>
                      <td>{part.filesystem || "—"}</td>
                      <td>—</td>
                      <td className="mono">{part.mountpoints.join(", ") || "—"}</td>
                    </tr>
                  )),
                ])}
              </tbody>
            </table>
          </div>
        </Panel>

        <Panel title={t("networkInterfaces")} className="wide">
          <div className="table-wrap">
            <table>
              <thead><tr><th>{t("name")}</th><th>{t("state")}</th><th>MAC</th><th>{t("addresses")}</th><th>{t("speed")}</th></tr></thead>
              <tbody>
                {value.system.network_interfaces.map((iface) => (
                  <tr key={iface.name}>
                    <td>{iface.name}</td>
                    <td>{status(iface.oper_state || (iface.up ? "up" : "down"))}</td>
                    <td className="mono">{iface.mac || "—"}</td>
                    <td className="mono">{iface.addresses.join(", ") || "—"}</td>
                    <td>{iface.speed_bps ? `${iface.speed_bps / 1_000_000} Mbps` : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Panel>
      </div>

    </div>
  );
}
