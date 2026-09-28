import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { Job, UpdateStatus } from "../api/types";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";

export function UpdatesPage({revision}: {revision: number}) {
  const {t, date} = useI18n();
  const [installing, setInstalling] = useState(false);
  const [checking, setChecking] = useState(false);
  const [manualUpdate, setManualUpdate] = useState<UpdateStatus>();
  const [lastChecked, setLastChecked] = useState<string>();
  const [installJob, setInstallJob] = useState<Job>();
  const [actionError, setActionError] = useState("");

  const load = useCallback(() => api.updates(), []);
  const resource = useResource(load, revision);

  async function checkUpdates() {
    setChecking(true);
    setActionError("");
    try {
      const result = await api.updates();
      setManualUpdate(result);
      setLastChecked(new Date().toISOString());
    } catch (error) {
      setActionError(error instanceof Error ? error.message : t("requestFailed"));
    } finally {
      setChecking(false);
    }
  }

  useEffect(() => {
    if (!installJob || !installing) return;

    let stopped = false;
    const target = manualUpdate?.available_version || resource.data?.available_version;
    const poll = async () => {
      if (stopped) return;
      try {
        const current = await api.job(installJob.id);
        if (stopped) return;
        setInstallJob(current);

        if (current.status === "failed" || current.status === "cancelled") {
          setInstalling(false);
          setActionError(current.error_message || current.message || t("requestFailed"));
          return;
        }

        if (current.status === "succeeded" && target) {
          const system = await api.system();
          if (system.version === target) {
            window.location.reload();
            return;
          }
        }
      } catch {
        // During package installation Core restarts. Verify the installed version
        // directly as soon as the API becomes reachable again.
        if (target) {
          try {
            const system = await api.system();
            if (system.version === target) {
              window.location.reload();
              return;
            }
          } catch {
            // Still restarting.
          }
        }
      }
      if (!stopped) window.setTimeout(poll, 1000);
    };

    poll();
    return () => {
      stopped = true;
    };
  }, [installJob?.id, installing, manualUpdate?.available_version, resource.data?.available_version, t]);

  async function install() {
    const target = manualUpdate?.available_version || resource.data?.available_version;
    if (!target) return;

    setInstalling(true);
    setActionError("");
    try {
      const job = await api.installUpdate(target);
      setInstallJob(job);
    } catch (error) {
      setInstalling(false);
      setActionError(error instanceof Error ? error.message : t("requestFailed"));
    }
  }


  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;
  const update = manualUpdate || resource.data!;

  return (
    <div className="page">
      <PageHeading title={t("updates")} subtitle={t("updatesSubtitle")} />

      <div className="two-column">
        <Panel
          title={t("updateStatus")}
          action={
            <button
              type="button"
              className="button secondary"
              disabled={checking || installing}
              onClick={checkUpdates}
            >
              {checking ? t("checking") : t("checkUpdates")}
            </button>
          }
        >
          <dl className="details">
            <dt>{t("currentVersion")}</dt>
            <dd className="mono">{update.current_version}</dd>
            <dt>{t("availableVersion")}</dt>
            <dd className="mono">{update.available_version || "—"}</dd>
            {update.published_at && <>
              <dt>{t("published")}</dt>
              <dd>{date(update.published_at)}</dd>
            </>}
          </dl>

          <div className={update.available ? "update-callout available" : "update-callout current"}>
            <strong>{update.available ? t("updateAvailable") : t("upToDate")}</strong>
            {update.available && <span>{t("updateRestartNotice")}</span>}
          </div>

          {actionError && <div className="form-error">{actionError}</div>}
          {lastChecked && <div className="notice">{t("lastChecked")}: {date(lastChecked)}</div>}
          {installJob && (
            <div className="notice">
              <div><strong>{installJob.message || t("installing")}</strong></div>
              <div className="progress"><span style={{width: `${Math.min(100, installJob.progress / 100)}%`}} /></div>
              <div className="small">
                {(installJob.progress / 100).toFixed(0)}% · {installJob.message || installJob.status}
              </div>
            </div>
          )}

          {update.available && (
            <button type="button" className="button primary" disabled={installing} onClick={install}>
              {installing ? t("installing") : t("installUpdate")}
            </button>
          )}
        </Panel>

        <Panel title={t("releaseNotes")}>
          {update.notes
            ? <pre className="release-notes">{update.notes}</pre>
            : <div className="muted">{t("noReleaseNotes")}</div>}
        </Panel>
      </div>
    </div>
  );
}
