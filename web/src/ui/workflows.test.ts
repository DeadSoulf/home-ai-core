import { describe, expect, it } from "vitest";
import { validateAuth } from "./authValidation";
import { browserLocale, errorText, uiMessages } from "./messages";
import { activeJob, canCancelJob, canUpgradeModule, catalogState, compareSemver, jobMessage, jobTitle, progressPercent, runtimeMessage } from "./moduleState";
import type { Job, ModuleRuntime, RegisteredModule } from "../api/types";
const manifest: RegisteredModule["manifest"] = {schema_version: 2, id: "demo", name: "Demo", version: "1.2.0", core: ">=0.1.0", runtime: {type: "docker", docker: {image: "example/image@sha256:" + "a".repeat(64)}}, lifecycle: ["install"]};
const installed: RegisteredModule = {manifest, status: "enabled"};
const job: Job = {id: "job-1", node_id: "node", type: "module.install", status: "running", progress: 1500, created_at: "2026-10-14T00:00:00Z"};
describe("UI-03 and UI-04: safe module action states", () => {
  it.each([
    ["1.1.0", "1.2.0", -1], ["1.10.0", "1.9.0", 1], ["1.0.0-rc.10", "1.0.0-rc.2", 1],
    ["1.0.0-alpha", "1.0.0", -1], ["1.0.0-alpha.1", "1.0.0-alpha", 1], ["1.0.0-1", "1.0.0-alpha", -1],
    ["1.2.3+a", "1.2.3+b", 0], ["1.2.3", "1.2.3", 0], ["99999999999999999999.0.0", "99999999999999999998.0.0", 1],
    ["1.01.0", "1.0.0", null], ["1.0.0-01", "1.0.0", null], ["dev", "1.0.0", null],
  ])("compares %s with %s", (a, b, result) => expect(compareSemver(a as string, b as string)).toBe(result));
  it("never offers a downgrade or unsupported upgrade", () => {
    expect(catalogState(installed, {...manifest, version: "1.1.0"})).toBe("older");
    expect(catalogState(installed, {...manifest, version: "1.3.0"})).toBe("newer");
    expect(catalogState(installed, manifest)).toBe("installed");
    expect(catalogState(undefined, manifest)).toBe("available");
  });
  it("offers catalog updates for any running upgrade-capable module", () => {
    for (const id of ["ai.agent", "nvr.module", "reference.module", "custom.module"]) {
      const current = {...installed, manifest: {...manifest, id, lifecycle: ["install", "upgrade", "remove"]}};
      const next = {...current.manifest, version: "1.3.0", runtime: {...current.manifest.runtime,
        docker: {image: "example/image@sha256:" + "b".repeat(64)}}};
      expect(canUpgradeModule(current, next)).toBe(true);
      expect(canUpgradeModule(current, undefined)).toBe(false);
      expect(canUpgradeModule(current, {...next, version: current.manifest.version})).toBe(false);
      expect(canUpgradeModule(current, {...next, version: "1.1.0"})).toBe(false);
      expect(canUpgradeModule(current, {...next, runtime: current.manifest.runtime})).toBe(false);
      expect(canUpgradeModule({...current, status: "disabled"}, next)).toBe(false);
      expect(canUpgradeModule({...current, status: "registered"}, next)).toBe(false);
      expect(canUpgradeModule(current, {...next, id: "other"})).toBe(false);
      expect(canUpgradeModule({...current, manifest: {...current.manifest, lifecycle: ["install"]}}, next)).toBe(false);
      expect(canUpgradeModule(current, {...next, lifecycle: ["install"]})).toBe(false);
    }
  });
  it("offers recovery independently of newer/changed catalog data", () => {
    expect(catalogState({...installed, status: "registered"}, {...manifest, version: "9.0.0"})).toBe("incomplete");
    expect(catalogState(installed, {...manifest, runtime: {...manifest.runtime, docker: {image: "different"}}})).toBe("changed");
  });
});
describe("UI-09, UI-11 and UI-14: truthful state presentation", () => {
  it("requires actual engine and bridge evidence", () => {
    const runtime: ModuleRuntime = {available: true, active: true, engine_ready: false, network_ready: false, version: "", checked_at: "now"};
    expect(runtimeMessage()).toBe("runtimeUnknown");
    expect(runtimeMessage(runtime)).toBe("engineUnavailable");
    expect(runtimeMessage({...runtime, engine_ready: true})).toBe("networkDegraded");
    expect(runtimeMessage({...runtime, engine_ready: true, network_ready: true})).toBe("runtimeReady");
  });
  it("only offers supported cancellation and localizes stages", () => {
    expect(activeJob(job)).toBe(true); expect(canCancelJob(job)).toBe(true);
    expect(canCancelJob({...job, status: "cancel_requested"})).toBe(false);
    expect(canCancelJob({...job, type: "core.update.install"})).toBe(false);
    expect(canCancelJob({...job, status: "queued", type: "other"})).toBe(true);
    expect(jobTitle(job)).toBe("moduleInstallJob");
    expect(jobMessage({...job, message: "pulling and creating module container"})).toBe("creating");
    expect(progressPercent({...job, progress: NaN})).toBe(0);
    expect(progressPercent({...job, progress: 20000})).toBe(100);
  });
  it("has identical keys and useful messages in both languages", () => {
    expect(Object.keys(uiMessages.ru).sort()).toEqual(Object.keys(uiMessages.en).sort());
    expect(errorText({code: "invalid_credentials"}, "ru")).toBe("Неверное имя пользователя или пароль.");
    expect(errorText({code: "invalid_bootstrap_token"}, "ru")).toContain("код");
    expect(errorText({status: 503}, "ru")).toBe("Сервер недоступен");
    expect(errorText(new TypeError("fetch failed"), "en")).toContain("Cannot reach");
    expect(["en", "ru"]).toContain(browserLocale());
  });
});
describe("UI-12: validation matches backend identity/password byte contracts", () => {
  it("does not submit missing login credentials", () => {
    expect(validateAuth({username: "owner", password: ""}, false)).toEqual({password: "required"});
    expect(validateAuth({username: ".ab", password: "123"}, false)).toHaveProperty("username");
  });
  it("does not apply new password minimum to legacy login", () => {
    expect(validateAuth({username: "owner", password: "old"}, false)).toEqual({});
  });
  it("counts UTF-8 bytes, not JavaScript UTF-16 units", () => {
    const fields = {username: "Owner", bootstrapToken: "test", password: "пароль", confirmation: "пароль"};
    expect(validateAuth(fields, true)).toEqual({});
    expect(validateAuth({...fields, password: "short", confirmation: "short"}, true)).toHaveProperty("password");
    expect(validateAuth({...fields, confirmation: "no"}, true)).toHaveProperty("confirmation");
    expect(validateAuth({...fields, password: "а".repeat(513)}, true)).toHaveProperty("password");
  });
});
