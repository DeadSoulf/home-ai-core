import { useState } from "react";
import type { UpdateStatus, UpdaterState } from "../api/types";
import { useI18n } from "../i18n";
import { useUI } from "../ui/useUI";
import { updateViewState } from "../ui/updateViewState";
import { ConfirmDialog } from "./ConfirmDialog";
import { Panel } from "./Panel";
import "./UpdateOverview.css";

export type UpdateOverviewProps = {
  installedVersion: string;
  architecture: string;
  canManage: boolean;
  status?: UpdateStatus;
  state?: UpdaterState;
  checking: boolean;
  downloading: boolean;
  installing: boolean;
  rollingBack: boolean;
  error: string;
  lastChecked?: string;
  onCheck: () => void;
  onDownload: () => void;
  onInstall: () => void;
  onRollback: () => void;
};

function formatUpdateBytes(value: number): string {
  if (value >= 1024 ** 3) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 3) + " GiB";
  if (value >= 1024 ** 2) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024 ** 2) + " MiB";
  if (value >= 1024) return new Intl.NumberFormat(undefined, {maximumFractionDigits: 1}).format(value / 1024) + " KiB";
  return value + " B";
}

export function UpdateOverview({
  installedVersion, architecture, canManage, status, state,
  checking, downloading, installing, rollingBack, error, lastChecked,
  onCheck, onDownload, onInstall, onRollback,
}: UpdateOverviewProps) {
  const {t, date} = useI18n();
  const u = useUI();
  const [confirmation, setConfirmation] = useState<"install" | "rollback" | null>(null);
  const view = updateViewState(status, state, {canManage, checking, downloading, installing, rollingBack});
  const target = status?.available ? status.available_version : undefined;
  const current = status?.current_version || installedVersion;
  const rollbackVersion = status?.rollback_version;

  return (
    <Panel
      className="wide updates-center"
      title={t("updateStatus")}
      action={<button type="button" className="button secondary" onClick={onCheck} disabled={!view.canCheck}>
        {checking ? t("checking") : t("checkUpdates")}
      </button>}
    >
      <p className="updates-intro">{u("coreUpdateIntro")}</p>

      <div className={"updates-status updates-status-" + view.phase} role="status" aria-live="polite">
        <span className="updates-status-mark" aria-hidden="true" />
        <div>
          <strong>{u(view.title)}</strong>
          <p>{u(view.hint)}</p>
        </div>
      </div>

      <div className="updates-versions">
        <div className="updates-version">
          <span>{t("currentVersion")}</span>
          <strong className="mono">{current}</strong>
          <small>{u("coreUpdateInstalled")}</small>
        </div>
        <span className="updates-version-arrow" aria-hidden="true">→</span>
        <div className="updates-version">
          <span>{t("availableVersion")}</span>
          <strong className="mono">{target || (status ? u("coreUpdateNoNewVersion") : "—")}</strong>
          <small>{target ? u("coreUpdateFromGitHub") : status ? u("coreUpdateNoNewHint") : u("coreUpdateCheckFirst")}</small>
        </div>
      </div>

      {view.showSteps && (
        <ol className="updates-steps" aria-label={u("coreUpdateSteps")}>
          {([
            {number: 1, label: u("coreUpdateStepCheck")},
            {number: 2, label: u("coreUpdateStepDownload")},
            {number: 3, label: u("coreUpdateStepInstall")},
          ] as const).map(({number, label}) => (
            <li key={number} className={number < view.step ? "complete" : number === view.step ? "active" : "pending"}
              aria-current={number === view.step ? "step" : undefined}>
              <span className="updates-step-number" aria-hidden="true">{number < view.step ? "✓" : number}</span>
              <span>{label}</span>
            </li>
          ))}
        </ol>
      )}

      {view.busy && view.phase !== "checking" && (
        <div className="updates-progress" role="status">
          <div className="updates-progress-heading">
            <strong>{u(view.title)}</strong>
            {view.progress !== null && <span>{view.progress}%</span>}
          </div>
          <progress max={100} value={view.progress === null ? undefined : view.progress}
            aria-label={u(view.title)} />
          <p>{u("coreUpdateDoNotRepeat")}</p>
        </div>
      )}

      {status?.helper_available === false && <div className="form-error" role="alert">{t("helperUnavailable")}</div>}
      {status?.helper_available && !status.helper_compatible &&
        <div className="form-error" role="alert">{t("helperUpgradeRequired")}</div>}
      {(error || state?.phase === "failed") &&
        <div className="form-error updates-error" role="alert">
          <strong>{u("coreUpdateErrorHeading")}</strong>
          <p>{error || state?.error || u("coreUpdateFailedHint")}</p>
          <p className="updates-subtle">{u("coreUpdateErrorNext")}</p>
        </div>}

      {canManage && !!target && !view.ready && !view.busy && (
        <div className="updates-primary-action">
          <button type="button" className="button primary" disabled={!view.canDownload} onClick={onDownload}>
            {t("downloadUpdate")}
          </button>
          <p>{u("coreUpdateDownloadNote")}</p>
        </div>
      )}
      {canManage && view.ready && !view.busy && (
        <div className="updates-primary-action">
          <button type="button" className="button primary" disabled={!view.canInstall}
            onClick={() => setConfirmation("install")}>{t("installUpdate")}</button>
          <p>{t("updateRestartNotice")}</p>
        </div>
      )}
      {!canManage && <p className="updates-subtle">{u("coreUpdateReadOnly")}</p>}

      {(status?.published_at || status?.bundle_size_bytes || lastChecked) &&
        <div className="updates-metadata">
          {status?.published_at && <span>{t("published")}: {date(status.published_at)}</span>}
          {!!status?.bundle_size_bytes && <span>{t("size")}: {formatUpdateBytes(status.bundle_size_bytes)}</span>}
          {lastChecked && <span>{t("lastChecked")}: {date(lastChecked)}</span>}
        </div>}

      <details className="updates-recovery">
        <summary>{u("coreUpdateRecoveryTitle")}</summary>
        <div className="updates-recovery-content">
          <p>{u("coreUpdateRecoveryHint")}</p>
          {status?.rollback_available && rollbackVersion ? (
            <>
              <dl className="details"><dt>{t("rollbackVersion")}</dt><dd className="mono">{rollbackVersion}</dd></dl>
              {canManage && (
                <button type="button" className="button danger" disabled={!view.canRollback}
                  onClick={() => setConfirmation("rollback")}>
                  {t("rollbackUpdate").replace("{version}", rollbackVersion)}
                </button>
              )}
              <p className="updates-subtle">{t("rollbackNotice")}</p>
            </>
          ) : <p className="updates-subtle">{u("coreUpdateNoRollback")}</p>}
        </div>
      </details>

      <details className="technical-details updates-technical">
        <summary>{t("technicalDetails")}</summary>
        <dl className="details">
          <dt>{t("architecture")}</dt><dd>{status?.architecture || architecture}</dd>
          <dt>{t("systemHelper")}</dt><dd>{status?.helper_version || "—"}</dd>
          <dt>{t("helperProtocol")}</dt><dd>{status?.helper_protocol || "—"}</dd>
          <dt>{t("state")}</dt><dd>{state?.phase || "—"}</dd>
          <dt>{t("progress")}</dt><dd>{state?.message || "—"}</dd>
        </dl>
      </details>
      {status?.notes && (
        <details className="technical-details updates-release-notes">
          <summary>{t("releaseNotes")}</summary><pre className="release-notes">{status.notes}</pre>
        </details>
      )}

      {confirmation === "install" && target && (
        <ConfirmDialog title={u("coreUpdateConfirmInstall")} confirmLabel={t("installUpdate")}
          onCancel={() => setConfirmation(null)}
          onConfirm={() => {setConfirmation(null); onInstall();}}>
          <p>{u("coreUpdateConfirmInstallHint").replace("{version}", target)}</p>
          <p>{t("updateRestartNotice")}</p>
        </ConfirmDialog>
      )}
      {confirmation === "rollback" && rollbackVersion && (
        <ConfirmDialog title={u("coreUpdateConfirmRollback")}
          confirmLabel={t("rollbackUpdate").replace("{version}", rollbackVersion)}
          onCancel={() => setConfirmation(null)}
          onConfirm={() => {setConfirmation(null); onRollback();}}>
          <p>{u("coreUpdateConfirmRollbackHint").replace("{version}", rollbackVersion)}</p>
          <p>{t("rollbackNotice")}</p>
        </ConfirmDialog>
      )}
    </Panel>
  );
}
