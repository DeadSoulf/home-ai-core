import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { PageHeading, Status } from "./Dashboard";

export function AuditPage({revision}: {revision: number}) {
  const load = useCallback(() => api.audit(), []);
  const {data, loading, error} = useResource(load, revision);

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;

  return (
    <div className="page">
      <PageHeading title="Audit" subtitle="Security-sensitive actions recorded by Core." />
      <Panel title="Recent events">
        <div className="table-wrap">
          <table>
            <thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>Target</th><th>Outcome</th></tr></thead>
            <tbody>
              {(data || []).map((entry) => (
                <tr key={entry.id}>
                  <td>{new Date(entry.occurred_at).toLocaleString()}</td>
                  <td>{entry.actor_id || entry.actor_type}</td>
                  <td className="mono">{entry.action}</td>
                  <td>{entry.target_type ? `${entry.target_type}: ${entry.target_id || "—"}` : "—"}</td>
                  <td><Status value={entry.outcome} /></td>
                </tr>
              ))}
              {(data || []).length === 0 && <tr><td colSpan={5} className="muted">No audit events yet.</td></tr>}
            </tbody>
          </table>
        </div>
      </Panel>
    </div>
  );
}
