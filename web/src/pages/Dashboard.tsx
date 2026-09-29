import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import type { BlockNode } from "../api/types";

function diskFreeBytes(root: BlockNode): number | undefined {
  const seen = new Set<string>();
  let filesystemFree = 0;
  let foundFilesystem = false;
  let unknownFilesystem = false;

  const visit = (node: BlockNode) => {
    const key = node.path || node.uuid || node.name;
    if (seen.has(key)) return;
    seen.add(key);

    const filesystem = (node.filesystem || "").toLowerCase();
    const dataFilesystem =
      filesystem !== "" &&
      filesystem !== "swap" &&
      filesystem !== "lvm2_member";

    if (dataFilesystem) {
      if (node.free_known) {
        filesystemFree += node.free_bytes || 0;
        foundFilesystem = true;
      } else {
        unknownFilesystem = true;
      }
    } else if (
      node.type !== "disk" &&
      node.type !== "lvm" &&
      (node.children || []).length === 0 &&
      !filesystem &&
      (node.size_bytes || 0) >= 64 * 1024 * 1024
    ) {
      // A sizeable leaf device with an unknown filesystem means we cannot
      // honestly calculate the disk's usable free space.
      unknownFilesystem = true;
    }

    for (const child of node.children || []) visit(child);
  };

  for (const child of root.children || []) visit(child);

  if (unknownFilesystem) return undefined;

  const unallocated = root.unallocated_bytes || 0;
  if (!foundFilesystem && unallocated === 0) return undefined;
  return filesystemFree + unallocated;
}

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
  const {t} = useI18n();
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
      <PageHeading title={t("dashboard")} subtitle={t("dashboardSubtitle")} />
      <div className="metric-grid">
        <Metric label={t("node")} value={data.system.system.hostname} detail={data.system.system.architecture} />
        <Metric label={t("cpu")} value={String(data.system.system.cpu.logical_cpus)} detail={data.system.system.cpu.model || t("logicalCpus")} />
        <Metric label={t("memory")} value={formatBytes(data.system.system.memory.total_bytes)} detail={`${formatBytes(data.system.system.memory.available_bytes)} ${t("available")}`} />
        <Metric label={t("modules")} value={String(data.modules.length)} detail={t("registered")} />
        <Metric label={t("jobs")} value={String(runningJobs)} detail={t("active")} />
        <Metric label={t("core")} value={data.system.version} detail={`${t("schema")} ${data.system.schema_version}`} />
      </div>

      <div className="two-column">
        <Panel title={t("storageOverview")}>
          <div className="list">
            {data.system.system.block_tree.filter((disk) => disk.type === "disk").length === 0 && (
              <span className="muted">{t("noBlockDevices")}</span>
            )}
            {data.system.system.block_tree
              .filter((disk) => disk.type === "disk")
              .slice(0, 6)
              .map((disk) => {
                const free = diskFreeBytes(disk);
                const health =
                  disk.health === "ok"
                    ? t("diskHealthOk")
                    : disk.health === "warning"
                      ? t("diskHealthWarning")
                      : disk.health === "failed"
                        ? t("diskHealthFailed")
                        : "";
                const details = [
                  disk.display_name ? disk.name : "",
                  t("freeSpace") + ": " + (free === undefined ? "—" : formatBytes(free)),
                  disk.temperature_c !== undefined ? disk.temperature_c + "°C" : "",
                  health,
                ].filter(Boolean).join(" · ");
                return (
                  <div className="list-row" key={disk.name}>
                    <div>
                      <strong>{disk.display_name || disk.model || disk.name}</strong>
                    </div>
                    <span>{details}</span>
                  </div>
                );
              })}
          </div>
        </Panel>

        <Panel title={t("recentJobs")}>
          <div className="list">
            {data.jobs.length === 0 && <span className="muted">{t("noJobs")}</span>}
            {data.jobs.slice(0, 6).map((job) => (
              <div className="list-row" key={job.id}>
                <div>
                  <strong>{job.type}</strong>
                  <span>{job.message || job.id}</span>
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
  const {status} = useI18n();
  return <span className={`status-badge status-${value.replaceAll("_", "-")}`}>{status(value)}</span>;
}
