import { useCallback, useState } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading, Status } from "./Dashboard";

export function JobsPage({revision}: {revision: number}) {
  const {t, date} = useI18n();
  const load = useCallback(() => api.jobs(), []);
  const {data, loading, error, reload} = useResource(load, revision);
  const [clearing, setClearing] = useState(false);
  const [actionError, setActionError] = useState("");

  async function clearHistory() {
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
      <PageHeading title={t("jobs")} subtitle={t("jobsSubtitle")} />
      <Panel
        title={t("jobHistory")}
        action={
          <button
            type="button"
            className="button secondary"
            disabled={clearing || !(data || []).some((job) => ["succeeded", "failed", "cancelled"].includes(job.status))}
            onClick={clearHistory}
          >
            {clearing ? t("clearingHistory") : t("clearHistory")}
          </button>
        }
      >
        {actionError && <div className="form-error">{actionError}</div>}
        <div className="table-wrap">
          <table>
            <thead><tr><th>{t("type")}</th><th>{t("status")}</th><th>{t("progress")}</th><th>{t("created")}</th><th>ID</th></tr></thead>
            <tbody>
              {(data || []).map((job) => (
                <tr key={job.id}>
                  <td><strong>{job.type}</strong><div className="muted small">{job.progress_message}</div></td>
                  <td><Status value={job.status} /></td>
                  <td>
                    <div className="progress"><span style={{width: `${Math.min(100, job.progress / 100)}%`}} /></div>
                    <span className="small">{(job.progress / 100).toFixed(0)}%</span>
                  </td>
                  <td>{date(job.created_at)}</td>
                  <td className="mono">{job.id}</td>
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
