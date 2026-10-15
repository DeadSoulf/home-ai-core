import type { UpdateStatus, UpdaterState } from "../api/types";
import type { UIKey } from "./messages";

export type UpdateScreenPhase =
  | "unknown" | "checking" | "current" | "available" | "ready"
  | "downloading" | "installing" | "rolling_back" | "restarting" | "failed";

export type UpdateViewFlags = {
  canManage: boolean;
  checking: boolean;
  downloading: boolean;
  installing: boolean;
  rollingBack: boolean;
};

export type UpdateViewModel = {
  phase: UpdateScreenPhase;
  title: UIKey;
  hint: UIKey;
  busy: boolean;
  helperBlocked: boolean;
  canCheck: boolean;
  canDownload: boolean;
  canInstall: boolean;
  canRollback: boolean;
  ready: boolean;
  step: 1 | 2 | 3;
  showSteps: boolean;
  progress: number | null;
};

const messages: Record<UpdateScreenPhase, {title: UIKey; hint: UIKey}> = {
  unknown: {title: "coreUpdateUnknown", hint: "coreUpdateUnknownHint"},
  checking: {title: "coreUpdateChecking", hint: "coreUpdateCheckingHint"},
  current: {title: "coreUpdateCurrent", hint: "coreUpdateCurrentHint"},
  available: {title: "coreUpdateAvailable", hint: "coreUpdateAvailableHint"},
  ready: {title: "coreUpdateReady", hint: "coreUpdateReadyHint"},
  downloading: {title: "coreUpdateDownloading", hint: "coreUpdateDownloadingHint"},
  installing: {title: "coreUpdateInstalling", hint: "coreUpdateInstallingHint"},
  rolling_back: {title: "coreUpdateRollingBack", hint: "coreUpdateRollingBackHint"},
  restarting: {title: "coreUpdateRestarting", hint: "coreUpdateRestartingHint"},
  failed: {title: "coreUpdateFailed", hint: "coreUpdateFailedHint"},
};

/** UI state is evidence-based: a stale ready phase must never install a different release. */
export function updateViewState(
  info: UpdateStatus | undefined,
  state: UpdaterState | undefined,
  flags: UpdateViewFlags,
): UpdateViewModel {
  const target = info?.available ? info.available_version : undefined;
  const ready = !!target && state?.phase === "ready" && state.available_version === target;
  const helperBlocked = info?.helper_available === false || info?.helper_compatible === false;
  const activeBackend = state?.phase === "downloading" || state?.phase === "installing" ||
    state?.phase === "rolling_back" || state?.phase === "restarting";
  const busy = flags.checking || flags.downloading || flags.installing || flags.rollingBack || activeBackend;

  let phase: UpdateScreenPhase;
  if (flags.rollingBack || state?.phase === "rolling_back") phase = "rolling_back";
  else if (state?.phase === "restarting") phase = "restarting";
  else if (flags.installing || state?.phase === "installing") phase = "installing";
  else if (flags.downloading || state?.phase === "downloading") phase = "downloading";
  else if (flags.checking) phase = "checking";
  else if (state?.phase === "failed") phase = "failed";
  else if (ready) phase = "ready";
  else if (target) phase = "available";
  else if (info) phase = "current";
  else phase = "unknown";

  const step: 1 | 2 | 3 =
    phase === "ready" || phase === "installing" || phase === "restarting" || phase === "rolling_back" ? 3 :
    phase === "downloading" ? 2 : 1;
  const progressValue = state?.progress_percent;
  const progress = busy && phase !== "checking" &&
    typeof progressValue === "number" && Number.isFinite(progressValue) ?
      Math.min(100, Math.max(0, progressValue)) : null;

  return {
    phase,
    ...messages[phase],
    busy,
    helperBlocked,
    canCheck: !busy,
    canDownload: flags.canManage && !!target && !ready && !busy && !helperBlocked,
    canInstall: flags.canManage && ready && !busy && !helperBlocked,
    canRollback: flags.canManage && !!info?.rollback_available && !!info.rollback_version && !busy && !helperBlocked,
    ready,
    step,
    showSteps: !!target || ready || flags.downloading || flags.installing || activeBackend,
    progress,
  };
}
