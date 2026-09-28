import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";

function bytes(value = 0) {
  return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
}

export function SystemPage({revision}: {revision: number}) {
  const {t, status} = useI18n();
  const load = useCallback(() => api.system(), []);
  const {data, loading, error} = useResource(load, revision);

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
            <dt>{t("ram")}</dt><dd>{bytes(value.system.memory.total_bytes)}</dd>
            <dt>{t("availableRam")}</dt><dd>{bytes(value.system.memory.available_bytes)}</dd>
            <dt>{t("gpuCount")}</dt><dd>{value.system.gpus.length}</dd>
            <dt>{t("gpu")}</dt><dd>{value.system.gpus.length ? value.system.gpus.map((gpu) => gpu.model || gpu.vendor || gpu.device_id || t("unknown")).join(", ") : "—"}</dd>
          </dl>
        </Panel>

        <Panel title={t("gpuDevices")} className="wide">
          <div className="table-wrap">
            <table>
              <thead><tr><th>{t("model")}</th><th>{t("vendor")}</th><th>{t("pciAddress")}</th><th>{t("deviceId")}</th><th>{t("driver")}</th></tr></thead>
              <tbody>
                {value.system.gpus.map((gpu) => (
                  <tr key={gpu.pci_address || gpu.card || gpu.device_id}>
                    <td>{gpu.model || t("unknown")}</td>
                    <td>{gpu.vendor || "—"}</td>
                    <td className="mono">{gpu.pci_address || "—"}</td>
                    <td className="mono">{gpu.vendor_id && gpu.device_id ? `${gpu.vendor_id}:${gpu.device_id}` : (gpu.device_id || "—")}</td>
                    <td>{gpu.driver || <span className="status-badge status-failed">{t("driverMissing")}</span>}</td>
                  </tr>
                ))}
                {value.system.gpus.length === 0 && <tr><td colSpan={5} className="muted">—</td></tr>}
              </tbody>
            </table>
          </div>
        </Panel>

        <Panel title={t("blockDevices")} className="wide">
          <div className="table-wrap">
            <table>
              <thead><tr><th>{t("device")}</th><th>{t("model")}</th><th>{t("size")}</th><th>{t("type")}</th><th>{t("serial")}</th></tr></thead>
              <tbody>
                {value.system.block_devices.map((disk) => (
                  <tr key={disk.name}>
                    <td className="mono">{disk.path}</td>
                    <td>{disk.model || "—"}</td>
                    <td>{bytes(disk.size_bytes)}</td>
                    <td>{disk.rotational ? "HDD" : t("flash")}</td>
                    <td className="mono">{disk.serial || "—"}</td>
                  </tr>
                ))}
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
