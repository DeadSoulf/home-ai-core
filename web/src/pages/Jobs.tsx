import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { PageHeading, Status } from "./Dashboard";

export function JobsPage({revision}: {revision: number}) {
  const load = useCallback(() => api.jobs(), []);
  const {data, loading, error} = useResource(load, revision);

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;

  return (
    <div className="page">
      <PageHeading title="Jobs" subtitle="Persistent background operations and progress." />
      <Panel title="Job history">
        <div className="table-wrap">
          <table>
            <thead><tr><th>Type</th><th>Status</th><th>Progress</th><th>Created</th><th>ID</th></tr></thead>
            <tbody>
              {(data || []).map((job) => (
                <tr key={job.id}>
                  <td><strong>{job.type}</strong><div className="muted small">{job.progress_message}</div></td>
                  <td><Status value={job.status} /></td>
                  <td>
                    <div className="progress"><span style={{width: `${Math.min(100, job.progress / 100)}%`}} /></div>
                    <span className="small">{(job.progress / 100).toFixed(0)}%</span>
                  </td>
                  <td>{new Date(job.created_at).toLocaleString()}</td>
                  <td className="mono">{job.id}</td>
                </tr>
              ))}
              {(data || []).length === 0 && <tr><td colSpan={5} className="muted">No jobs yet.</td></tr>}
            </tbody>
          </table>
        </div>
      </Panel>
    </div>
  );
}
