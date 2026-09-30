import { useCallback } from "react";
import type { Actor } from "../api/types";
import { useResource } from "../hooks/useResource";
import { EmptyState, ErrorState, LoadingState, Panel } from "../components/Panel";
import { jobLabelKey } from "../display";
import { useI18n } from "../i18n";
import { loadOverview } from "./overview";

function formatBytes(value?: number): string {
  if (value === undefined) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index++; }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

export function Dashboard({actor, revision, onNavigate}: {
  actor: Actor; revision: number; onNavigate: (path: string) => void;
}) {
  const {t, date} = useI18n();
  const load = useCallback(() => loadOverview(actor), [actor]);
  const resource = useResource(load, revision);
  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;
  const data = resource.data!;
  const activeJobs = data.jobs?.filter((job) => ["queued", "running", "cancel_requested"].includes(job.status));
  const moduleErrors = data.modules?.filter((module) => module.status === "error") || [];
  const missingDrivers = data.system?.system.gpus.filter((gpu) => !gpu.driver) || [];
  const sectionLabels = { system: "equipment", modules: "modules", jobs: "activity" } as const;
  const uptime = data.system?.system.uptime_seconds;
  const uptimeDetail = uptime === undefined ? t("equipment") : `${t("uptime")}: ${Math.floor(uptime / 86400)} ${t("days")} ${Math.floor(uptime % 86400 / 3600)} ${t("hours")}`;
  const hasData = data.system || data.modules || data.jobs;

  return (
    <div className="page">
      <PageHeading title={t("home")} subtitle={t("dashboardSubtitleSimple")} />
      {data.errors.map((error) => <ErrorState key={error.section} message={`${t(sectionLabels[error.section])}: ${error.message}`} />)}
      {!hasData && data.errors.length === 0 && <EmptyState>{t("noOverviewAccess")}</EmptyState>}
      {hasData && <div className="metric-grid overview-metrics">
        {data.system && <>
          <Metric label={t("serverName")} value={data.system.system.hostname} detail={uptimeDetail} />
          <Metric label={t("memory")} value={formatBytes(data.system.system.memory.total_bytes)} detail={`${t("availableRam")}: ${formatBytes(data.system.system.memory.available_bytes)}`} />
        </>}
        {data.modules && <Metric label={t("modules")} value={String(data.modules.length)} detail={`${t("active")}: ${data.modules.filter((module) => module.status === "enabled").length}`} />}
        {activeJobs && <Metric label={t("activeTasks")} value={String(activeJobs.length)} detail={t("backgroundOperations")} />}
      </div>}
      {(moduleErrors.length > 0 || missingDrivers.length > 0) && <Panel title={t("needsAttention")}>
        <div className="list">
          {moduleErrors.map((module) => <div className="list-row" key={module.manifest.id}>
            <div><strong>{module.manifest.name}</strong><span>{module.error || t("moduleError")}</span></div>
            <button className="text-button" onClick={() => onNavigate("/modules")}>{t("modules")}</button>
          </div>)}
          {missingDrivers.map((gpu, index) => <div className="list-row" key={gpu.pci_address || index}>
            <div><strong>{gpu.model || t("gpu")}</strong><span>{t("driverMissing")}</span></div>
            <button className="text-button" onClick={() => onNavigate("/system")}>{t("equipment")}</button>
          </div>)}
        </div>
      </Panel>}
      <div className="two-column">
        {data.modules && <Panel title={t("modules")} action={<button className="text-button" onClick={() => onNavigate("/modules")}>{t("showAll")}</button>}>
          <div className="list">
            {data.modules.length === 0 && <span className="muted">{t("noModulesSimple")}</span>}
            {data.modules.slice(0, 4).map((module) => <div className="list-row" key={module.manifest.id}>
              <div><strong>{module.manifest.name}</strong></div><Status value={module.status} />
            </div>)}
          </div>
        </Panel>}
        {data.jobs && <Panel title={t("activity")} action={<button className="text-button" onClick={() => onNavigate("/jobs")}>{t("showAll")}</button>}>
          <div className="list">
            {data.jobs.length === 0 && <span className="muted">{t("noJobs")}</span>}
            {[...data.jobs].sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, 4).map((job) => <div className="list-row" key={job.id}>
              <div><strong>{t(jobLabelKey(job.type))}</strong><span>{date(job.created_at)}</span></div><Status value={job.status} />
            </div>)}
          </div>
        </Panel>}
      </div>
    </div>
  );
}

export function PageHeading({title, subtitle}: {title: string; subtitle: string}) {
  return <header className="page-heading"><div><h1>{title}</h1><p>{subtitle}</p></div></header>;
}

export function Metric({label, value, detail}: {label: string; value: string; detail: string}) {
  return <div className="metric"><span>{label}</span><strong title={value}>{value}</strong><small>{detail}</small></div>;
}

export function Status({value}: {value: string}) {
  const {status} = useI18n();
  return <span className={`status-badge status-${value.replaceAll("_", "-")}`}>{status(value)}</span>;
}
