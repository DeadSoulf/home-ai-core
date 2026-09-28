import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading, Status } from "./Dashboard";

export function AuditPage({revision}: {revision: number}) {
  const {t, date} = useI18n();
  const load = useCallback(() => api.audit(), []);
  const {data, loading, error} = useResource(load, revision);

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;

  return (
    <div className="page">
      <PageHeading title={t("audit")} subtitle={t("auditSubtitle")} />
      <Panel title={t("recentEvents")}>
        <div className="table-wrap">
          <table>
            <thead><tr><th>{t("time")}</th><th>{t("actor")}</th><th>{t("action")}</th><th>{t("target")}</th><th>{t("outcome")}</th></tr></thead>
            <tbody>
              {(data || []).map((entry) => (
                <tr key={entry.id}>
                  <td>{date(entry.occurred_at)}</td>
                  <td>{entry.actor_id || entry.actor_type}</td>
                  <td className="mono">{entry.action}</td>
                  <td>{entry.target_type ? `${entry.target_type}: ${entry.target_id || "—"}` : "—"}</td>
                  <td><Status value={entry.outcome} /></td>
                </tr>
              ))}
              {(data || []).length === 0 && <tr><td colSpan={5} className="muted">{t("noAudit")}</td></tr>}
            </tbody>
          </table>
        </div>
      </Panel>
    </div>
  );
}
