import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { actorLabelKey, auditLabelKey } from "../display";
import { PageHeading, Status } from "./Dashboard";

export function AuditPage({revision}: {revision: number}) {
  const {t, date} = useI18n();
  const load = useCallback(() => api.audit(), []);
  const {data, loading, error} = useResource(load, revision);

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;

  return (
    <div className="page">
      <PageHeading title={t("securityLog")} subtitle={t("auditSubtitleSimple")} />
      {error && <ErrorState message={error} />}
      <Panel title={t("recentEvents")}>
        <div className="table-wrap">
          <table>
            <thead><tr><th>{t("time")}</th><th>{t("action")}</th><th>{t("actor")}</th><th>{t("outcome")}</th><th>{t("technicalDetails")}</th></tr></thead>
            <tbody>
              {(data || []).map((entry) => (
                <tr key={entry.id}>
                  <td>{date(entry.occurred_at)}</td>
                  <td><strong>{t(auditLabelKey(entry.action))}</strong></td>
                  <td>{t(actorLabelKey(entry.actor_type))}</td>
                  <td><Status value={entry.outcome} /></td>
                  <td>
                    <details className="technical-details row-details">
                      <summary>{t("technicalDetails")}</summary>
                      <dl className="details">
                        <dt>ID</dt><dd className="mono">{entry.id}</dd>
                        <dt>{t("action")}</dt><dd className="mono">{entry.action}</dd>
                        <dt>{t("actor")}</dt><dd className="mono">{entry.actor_type}{entry.actor_id ? `: ${entry.actor_id}` : ""}</dd>
                        <dt>{t("target")}</dt><dd className="mono">{entry.target_type ? `${entry.target_type}: ${entry.target_id || "—"}` : "—"}</dd>
                        {entry.request_id && <><dt>{t("requestId")}</dt><dd className="mono">{entry.request_id}</dd></>}
                        {entry.correlation_id && <><dt>{t("correlationId")}</dt><dd className="mono">{entry.correlation_id}</dd></>}
                      </dl>
                      {Object.keys(entry.metadata || {}).length > 0 && <pre className="mono" aria-label={t("auditMetadata")}>{JSON.stringify(entry.metadata, null, 2)}</pre>}
                    </details>
                  </td>
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
