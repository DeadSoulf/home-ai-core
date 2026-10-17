import { useCallback, useEffect, useRef, useState } from "react";
import type { Job } from "../api/types";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { ErrorNotice } from "../components/ErrorNotice";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { JobProgress } from "../components/JobProgress";
import { SectionGuide } from "../components/SectionGuide";
import { useI18n } from "../i18n";
import { useUI } from "../ui/useUI";
import { activeJob, canCancelJob, jobTitle } from "../ui/moduleState";
import { PageHeading, Status } from "./Dashboard";

export function JobsPage({revision, canManage = false, selectedJob}: {revision: number; canManage?: boolean; selectedJob?: string}) {
  const {t, date} = useI18n(); const u = useUI();
  const load = useCallback(async (signal: AbortSignal) => {
    const jobs = await api.jobs(signal);
    if (selectedJob && !jobs.some((job) => job.id === selectedJob)) {
      try { jobs.unshift(await api.job(selectedJob, signal)); } catch (error) { if ((error as {status?: number}).status !== 404) throw error; }
    }
    return jobs;
  }, [selectedJob]);
  const {data, loading, error, reload} = useResource(load, revision, 2000);
  const [clearing, setClearing] = useState(false), [cancelling, setCancelling] = useState("");
  const [actionError, setActionError] = useState<unknown>(), [cancelTarget, setCancelTarget] = useState<Job>();
  const lock = useRef(false);
  const [cancelAcknowledged, setCancelAcknowledged] = useState<Set<string>>(() => new Set());
  const visibleJobs = data?.map((job) => cancelAcknowledged.has(job.id) && activeJob(job) ? {...job, status: "cancel_requested"} : job);
  const selectedExists = Boolean(selectedJob && data?.some((job) => job.id === selectedJob));
  useEffect(() => { if (selectedExists) document.getElementById("job-" + selectedJob)?.focus(); }, [selectedJob, selectedExists]);
  async function clearHistory() {
    if (!canManage || lock.current) return;
    lock.current = true; setClearing(true); setActionError(undefined);
    try { await api.clearJobs(); reload(); }
    catch (reason) { setActionError(reason); }
    finally { lock.current = false; setClearing(false); }
  }
  async function cancel(job: Job) {
    if (!canManage || lock.current || !canCancelJob(job)) return;
    lock.current = true; setCancelling(job.id); setActionError(undefined); setCancelTarget(undefined);
    try { await api.cancelJob(job.id); setCancelAcknowledged((ids) => new Set([...ids, job.id])); }
    catch (reason) { setActionError(reason); }
    finally { lock.current = false; setCancelling(""); reload(); }
  }
  if (loading && !data) return <LoadingState />;
  if (!data) return <ErrorState message={error} onRetry={reload} />;
  return <div className="page jobs-page">
    <PageHeading title={t("activity")} subtitle={t("jobsSubtitleSimple")} />
    <SectionGuide title={u("uxJobsGuide")} description={u("uxJobsGuideDescription")} />
    {error && <ErrorState message={error} onRetry={reload} />}
    {actionError !== undefined && <ErrorNotice error={actionError} />}
    <Panel title={t("jobHistory")} action={canManage && <button type="button" className="button secondary" disabled={clearing || Boolean(cancelling) || !data.some((job) => !activeJob(job))}
      onClick={() => void clearHistory()}>{t(clearing ? "clearingHistory" : "clearHistory")}</button>}>
      <div className="table-wrap"><table>
        <thead><tr><th>{t("task")}</th><th>{t("status")}</th><th>{t("progress")}</th><th>{t("created")}</th><th>{u("action")}</th><th>{u("details")}</th></tr></thead>
        <tbody>{visibleJobs!.map((job) => <tr key={job.id} id={"job-" + job.id} tabIndex={-1} className={job.id === selectedJob ? "selected-job" : undefined}>
          <td><strong>{u(jobTitle(job))}</strong>
            {job.input?.module_id && <p className="mono small">{u("module")}: {job.input.module_id}</p>}
            {job.input?.operation && <p className="small">{({enable: t("moduleStart"), disable: t("moduleStop"), restart: t("moduleRestart")} as Record<string,string>)[job.input.operation] || job.input.operation}</p>}
            {job.error_message && <p className="form-error">{u("jobFailed")}</p>}
          </td>
          <td><Status value={job.status} /></td><td><JobProgress job={job} /></td><td>{date(job.created_at)}</td>
          <td>{canManage && canCancelJob(job) && <button type="button" className="button secondary" disabled={Boolean(cancelling) || clearing} onClick={() => setCancelTarget(job)}>{u("cancelJob")}</button>}
            {job.status === "cancel_requested" && <span role="status">{u("cancelling")}</span>}
            {canManage && job.status === "running" && !canCancelJob(job) && <span className="small muted">{u("cannotCancel")}</span>}
          </td>
          <td><details className="technical-details row-details"><summary>{u("details")}</summary>
            <dl className="details"><dt>ID</dt><dd className="mono">{job.id}</dd><dt>{t("type")}</dt><dd className="mono">{job.type}</dd>
              <dt>{t("nodeId")}</dt><dd className="mono">{job.node_id}</dd>
              {job.message && <><dt>{t("status")}</dt><dd>{job.message}</dd></>}
              {job.error_message && <><dt>{u("failed")}</dt><dd>{job.error_message}</dd></>}
              {job.request_id && <><dt>{t("requestId")}</dt><dd className="mono">{job.request_id}</dd></>}
              {job.correlation_id && <><dt>{t("correlationId")}</dt><dd className="mono">{job.correlation_id}</dd></>}
            </dl>
          </details></td>
        </tr>)}{data.length === 0 && <tr><td colSpan={6} className="muted">{t("noJobs")}</td></tr>}</tbody>
      </table></div>
    </Panel>
    {cancelTarget && <ConfirmDialog title={u("cancelTitle")} confirmLabel={u("cancelJob")} onCancel={() => setCancelTarget(undefined)} onConfirm={() => void cancel(cancelTarget)}>
      <p><strong>{u(jobTitle(cancelTarget))}</strong> <span className="mono">{cancelTarget.id}</span></p><p>{u("cancelHint")}</p>
    </ConfirmDialog>}
  </div>;
}
