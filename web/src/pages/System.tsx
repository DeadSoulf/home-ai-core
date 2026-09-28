import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { PageHeading } from "./Dashboard";

function bytes(value = 0) {
  return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
}

export function SystemPage({revision}: {revision: number}) {
  const load = useCallback(() => api.system(), []);
  const {data, loading, error} = useResource(load, revision);

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;
  const value = data!;

  return (
    <div className="page">
      <PageHeading title="System" subtitle="Read-only hardware and operating-system inventory." />
      <div className="two-column">
        <Panel title="Node">
          <dl className="details">
            <dt>Hostname</dt><dd>{value.system.hostname}</dd>
            <dt>Node ID</dt><dd className="mono">{value.system.node_id}</dd>
            <dt>OS</dt><dd>{value.system.os}</dd>
            <dt>Kernel</dt><dd>{value.system.kernel || "—"}</dd>
            <dt>Architecture</dt><dd>{value.system.architecture}</dd>
            <dt>Core version</dt><dd>{value.version}</dd>
          </dl>
        </Panel>

        <Panel title="Compute">
          <dl className="details">
            <dt>CPU</dt><dd>{value.system.cpu.model || "Unknown"}</dd>
            <dt>Logical CPUs</dt><dd>{value.system.cpu.logical_cpus}</dd>
            <dt>RAM</dt><dd>{bytes(value.system.memory.total_bytes)}</dd>
            <dt>Available RAM</dt><dd>{bytes(value.system.memory.available_bytes)}</dd>
            <dt>GPU count</dt><dd>{value.system.gpus.length}</dd>
          </dl>
        </Panel>

        <Panel title="Block devices" className="wide">
          <div className="table-wrap">
            <table>
              <thead><tr><th>Device</th><th>Model</th><th>Size</th><th>Type</th><th>Serial</th></tr></thead>
              <tbody>
                {value.system.block_devices.map((disk) => (
                  <tr key={disk.name}>
                    <td className="mono">{disk.path}</td>
                    <td>{disk.model || "—"}</td>
                    <td>{bytes(disk.size_bytes)}</td>
                    <td>{disk.rotational ? "HDD" : "SSD / flash"}</td>
                    <td className="mono">{disk.serial || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Panel>

        <Panel title="Network interfaces" className="wide">
          <div className="table-wrap">
            <table>
              <thead><tr><th>Name</th><th>State</th><th>MAC</th><th>Addresses</th><th>Speed</th></tr></thead>
              <tbody>
                {value.system.network_interfaces.map((iface) => (
                  <tr key={iface.name}>
                    <td>{iface.name}</td>
                    <td>{iface.oper_state || (iface.up ? "up" : "down")}</td>
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
