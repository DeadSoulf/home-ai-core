import { useCallback, useState } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { jobLabelKey } from "../display";
import { PageHeading, Status } from "./Dashboard";

export function JobsPage({revision, canManage = false}: {revision: number; canManage?: boolean}) {
  const {t, date} = useI18n();
  const load = useCallback(() => api.jobs(), []);
  const {data, loading, error, reload} = useResource(load, revision);
  const [clearing, setClearing] = useState(false);
  const [actionError, setActionError] = useState("");

  async function clearHistory() {
    if (!canManage || clearing) return;
    setClearing(true);
    setActionError("");
    try {
      await api.clearJobs();
      reload();
    } catch (reason) {
      setActionError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setClearing(false);
    }
  }

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;

  return (
    <div className="page">
      <PageHeading title={t("activity")} subtitle={t("jobsSubtitleSimple")} />
      {error && <ErrorState message={error} />}
      <Panel
        title={t("jobHistory")}
        action={canManage && (
          <button
            type="button"
            className="button secondary"
            disabled={clearing || !(data || []).some((job) => ["succeeded", "failed", "cancelled"].includes(job.status))}
            onClick={clearHistory}
          >
            {clearing ? t("clearingHistory") : t("clearHistory")}
          </button>
        )}
      >
        {actionError && <div className="form-error">{actionError}</div>}
        <div className="table-wrap">
          <table>
            <thead><tr><th>{t("task")}</th><th>{t("status")}</th><th>{t("progress")}</th><th>{t("created")}</th><th>{t("technicalDetails")}</th></tr></thead>
            <tbody>
              {(data || []).map((job) => (
                <tr key={job.id}>
                  <td>
                    <strong>{t(jobLabelKey(job.type))}</strong>
                    {job.message && <div className="muted small">{job.message}</div>}
                    {job.error_message && <div className="form-error">{job.error_message}</div>}
                  </td>
                  <td><Status value={job.status} /></td>
                  <td>
                    <div className="progress"><span style={{width: `${Math.max(0, Math.min(100, job.progress / 100))}%`}} /></div>
                    <span className="small">{Math.max(0, Math.min(100, job.progress / 100)).toFixed(0)}%</span>
                  </td>
                  <td>{date(job.created_at)}</td>
                  <td>
                    <details className="technical-details row-details">
                      <summary>{t("technicalDetails")}</summary>
                      <dl className="details">
                        <dt>ID</dt><dd className="mono">{job.id}</dd>
                        <dt>{t("type")}</dt><dd className="mono">{job.type}</dd>
                        <dt>{t("nodeId")}</dt><dd className="mono">{job.node_id}</dd>
                        {job.actor_type && <><dt>{t("actor")}</dt><dd className="mono">{job.actor_type}{job.actor_id ? `: ${job.actor_id}` : ""}</dd></>}
                        {job.request_id && <><dt>{t("requestId")}</dt><dd className="mono">{job.request_id}</dd></>}
                        {job.correlation_id && <><dt>{t("correlationId")}</dt><dd className="mono">{job.correlation_id}</dd></>}
                      </dl>
                    </details>
                  </td>
                </tr>
              ))}
              {(data || []).length === 0 && <tr><td colSpan={5} className="muted">{t("noJobs")}</td></tr>}
            </tbody>
          </table>
        </div>
      </Panel>
    </div>
  );
}
