import { useCallback, useState } from "react";
import { api } from "../api/client";
import type { Job } from "../api/types";
import { ErrorState, LoadingState, Panel } from "../components/Panel";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";

export function UpdatesPage({revision}: {revision: number}) {
  const {t, date} = useI18n();
  const [refresh, setRefresh] = useState(0);
  const [installing, setInstalling] = useState(false);
  const [installJob, setInstallJob] = useState<Job>();
  const [actionError, setActionError] = useState("");

  const load = useCallback(() => api.updates(), []);
  const resource = useResource(load, revision + refresh);

  async function install() {
    const target = resource.data?.available_version;
    if (!target) return;

    setInstalling(true);
    setActionError("");
    try {
      const job = await api.installUpdate();
      setInstallJob(job);
      waitForVersion(target);
    } catch (error) {
      setInstalling(false);
      setActionError(error instanceof Error ? error.message : t("requestFailed"));
    }
  }

  function waitForVersion(target: string) {
    const deadline = Date.now() + 10 * 60 * 1000;
    const check = async () => {
      try {
        const system = await api.system();
        if (system.version === target) {
          window.location.reload();
          return;
        }
      } catch {
        // Core is expected to be briefly unavailable during package restart.
      }
      if (Date.now() < deadline) {
        window.setTimeout(check, 2000);
      } else {
        setInstalling(false);
      }
    };
    window.setTimeout(check, 2000);
  }

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;
  const update = resource.data!;

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
              disabled={resource.loading || installing}
              onClick={() => setRefresh((value) => value + 1)}
            >
              {resource.loading ? t("checking") : t("checkUpdates")}
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
          {installJob && <div className="notice">{t("updateStarted")} <span className="mono">{installJob.id}</span></div>}

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
