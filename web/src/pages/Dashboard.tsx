import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";

function formatBytes(value = 0): string {
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index++;
  }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

export function Dashboard({revision}: {revision: number}) {
  const load = useCallback(async () => {
    const [system, modules, jobs] = await Promise.all([
      api.system(),
      api.modules(),
      api.jobs(),
    ]);
    return {system, modules, jobs};
  }, []);
  const resource = useResource(load, revision);

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;
  const data = resource.data!;

  const runningJobs = data.jobs.filter((job) =>
    ["queued", "running", "cancel_requested"].includes(job.status),
  ).length;

  return (
    <div className="page">
      <PageHeading title="Dashboard" subtitle="Current state of this Home-AI-Core node." />
      <div className="metric-grid">
        <Metric label="Node" value={data.system.system.hostname} detail={data.system.system.architecture} />
        <Metric label="CPU" value={String(data.system.system.cpu.logical_cpus)} detail={data.system.system.cpu.model || "logical CPUs"} />
        <Metric label="Memory" value={formatBytes(data.system.system.memory.total_bytes)} detail={`${formatBytes(data.system.system.memory.available_bytes)} available`} />
        <Metric label="Modules" value={String(data.modules.length)} detail="registered" />
        <Metric label="Jobs" value={String(runningJobs)} detail="active" />
        <Metric label="Core" value={data.system.version} detail={`schema ${data.system.schema_version}`} />
      </div>

      <div className="two-column">
        <Panel title="Storage overview">
          <div className="list">
            {data.system.system.block_devices.length === 0 && <span className="muted">No block devices reported.</span>}
            {data.system.system.block_devices.slice(0, 6).map((disk) => (
              <div className="list-row" key={disk.name}>
                <div>
                  <strong>{disk.model || disk.name}</strong>
                  <span>{disk.path}</span>
                </div>
                <span>{formatBytes(disk.size_bytes)}</span>
              </div>
            ))}
          </div>
        </Panel>

        <Panel title="Recent jobs">
          <div className="list">
            {data.jobs.length === 0 && <span className="muted">No jobs yet.</span>}
            {data.jobs.slice(0, 6).map((job) => (
              <div className="list-row" key={job.id}>
                <div>
                  <strong>{job.type}</strong>
                  <span>{job.progress_message || job.id}</span>
                </div>
                <Status value={job.status} />
              </div>
            ))}
          </div>
        </Panel>
      </div>
    </div>
  );
}

export function PageHeading({title, subtitle}: {title: string; subtitle: string}) {
  return (
    <header className="page-heading">
      <div>
        <h1>{title}</h1>
        <p>{subtitle}</p>
      </div>
    </header>
  );
}

export function Metric({label, value, detail}: {label: string; value: string; detail: string}) {
  return (
    <div className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{detail}</small>
    </div>
  );
}

export function Status({value}: {value: string}) {
  return <span className={`status-badge status-${value.replaceAll("_", "-")}`}>{value}</span>;
}
