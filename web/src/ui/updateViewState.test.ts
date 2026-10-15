import { describe, expect, it } from "vitest";
import type { UpdateStatus, UpdaterState } from "../api/types";
import { uiMessages } from "./messages";
import { updateViewState, type UpdateViewFlags } from "./updateViewState";

const status: UpdateStatus = {
  current_version: "0.1.174-dev",
  available_version: "0.1.175-dev",
  available: true,
  architecture: "amd64",
  helper_available: true,
  helper_compatible: true,
  rollback_available: true,
  rollback_version: "0.1.173-dev",
};

const flags: UpdateViewFlags = {
  canManage: true,
  checking: false,
  downloading: false,
  installing: false,
  rollingBack: false,
};

const ready: UpdaterState = {
  phase: "ready",
  available_version: "0.1.175-dev",
  updated_at: "2026-10-15T00:00:00Z",
};

describe("guided Core updater interface", () => {
  it("does not invent update status before a successful check", () => {
    const view = updateViewState(undefined, undefined, flags);
    expect(view.phase).toBe("unknown");
    expect(view.canCheck).toBe(true);
    expect(view.canDownload).toBe(false);
    expect(view.canInstall).toBe(false);
    expect(view.canRollback).toBe(false);
    expect(view.showSteps).toBe(false);
  });

  it("does not offer installation when no published update exists", () => {
    const view = updateViewState({...status, available: false}, ready, flags);
    expect(view.phase).toBe("current");
    expect(view.canDownload).toBe(false);
    expect(view.canInstall).toBe(false);
    expect(view.canRollback).toBe(true);
  });

  it("offers download first and requires a matching downloaded version for installation", () => {
    const available = updateViewState(status, undefined, flags);
    expect(available.phase).toBe("available");
    expect(available.canDownload).toBe(true);
    expect(available.canInstall).toBe(false);
    expect(available.step).toBe(1);

    const stale = updateViewState(status, {...ready, available_version: "0.1.174-dev"}, flags);
    expect(stale.canDownload).toBe(true);
    expect(stale.canInstall).toBe(false);

    const verified = updateViewState(status, ready, flags);
    expect(verified.phase).toBe("ready");
    expect(verified.canDownload).toBe(false);
    expect(verified.canInstall).toBe(true);
    expect(verified.step).toBe(3);
  });

  it("disables sensitive operations while an update is underway", () => {
    for (const phase of ["downloading", "installing", "rolling_back", "restarting"] as const) {
      const view = updateViewState(status, {...ready, phase}, flags);
      expect(view.phase).toBe(phase);
      expect(view.busy).toBe(true);
      expect(view.canCheck).toBe(false);
      expect(view.canDownload).toBe(false);
      expect(view.canInstall).toBe(false);
      expect(view.canRollback).toBe(false);
    }
  });

  it("does not claim that cancelling or losing a request proves failure", () => {
    const view = updateViewState(status, ready, {...flags, installing: true});
    expect(view.phase).toBe("installing");
    expect(view.busy).toBe(true);
    expect(view.canInstall).toBe(false);
    expect(view.canRollback).toBe(false);
  });

  it("blocks operations when helper is absent or incompatible", () => {
    for (const patch of [{helper_available: false}, {helper_compatible: false}]) {
      const info = {...status, ...patch};
      const view = updateViewState(info, ready, flags);
      expect(view.helperBlocked).toBe(true);
      expect(view.canInstall).toBe(false);
      expect(view.canRollback).toBe(false);
      expect(updateViewState(info, undefined, flags).canDownload).toBe(false);
    }
  });

  it("keeps read-only users from changing the server", () => {
    const view = updateViewState(status, ready, {...flags, canManage: false});
    expect(view.canInstall).toBe(false);
    expect(view.canRollback).toBe(false);
    expect(view.canCheck).toBe(true);
    expect(updateViewState(status, undefined, {...flags, canManage: false}).canDownload).toBe(false);
  });

  it("never displays impossible progress percentages", () => {
    expect(updateViewState(status, {...ready, phase: "downloading", progress_percent: 199}, flags).progress).toBe(100);
    expect(updateViewState(status, {...ready, phase: "downloading", progress_percent: -4}, flags).progress).toBe(0);
    expect(updateViewState(status, {...ready, phase: "downloading", progress_percent: NaN}, flags).progress).toBeNull();
    expect(updateViewState(status, ready, flags).progress).toBeNull();
  });

  it("retains failure state and manual action safety", () => {
    const view = updateViewState(status, {...ready, phase: "failed", error: "disk full"}, flags);
    expect(view.phase).toBe("failed");
    expect(view.canInstall).toBe(false);
    expect(view.canCheck).toBe(true);
  });

  it("has localized guidance for every possible status", () => {
    for (const state of ["idle", "checking", "available", "ready", "downloading", "installing", "rolling_back", "restarting", "succeeded", "failed"] as const) {
      const view = updateViewState(status, {...ready, phase: state}, flags);
      expect(uiMessages.en[view.title]).toBeTruthy();
      expect(uiMessages.en[view.hint]).toBeTruthy();
      expect(uiMessages.ru[view.title]).toBeTruthy();
      expect(uiMessages.ru[view.hint]).toBeTruthy();
    }
    expect(Object.keys(uiMessages.ru).sort()).toEqual(Object.keys(uiMessages.en).sort());
  });
});
