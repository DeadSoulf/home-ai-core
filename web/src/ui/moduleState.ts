import type { Job, RegisteredModule, ModuleRuntime } from "../api/types";
import type { UIKey } from "./messages";
function parseSemver(value: string) {
  const match = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/.exec(value);
  if (!match) return undefined;
  const pre = match[4]?.split(".") || [];
  if (pre.some((item) => /^0[0-9]+$/.test(item))) return undefined;
  return {main: match.slice(1, 4), pre};
}
function numeric(a: string, b: string) { return a.length === b.length ? (a < b ? -1 : a > b ? 1 : 0) : a.length < b.length ? -1 : 1; }
// SemVer precedence, including numeric prerelease identifiers and large values.
export function compareSemver(a: string, b: string): number | null {
  const left = parseSemver(a), right = parseSemver(b);
  if (!left || !right) return null;
  for (let i = 0; i < 3; i++) { const order = numeric(left.main[i], right.main[i]); if (order) return order; }
  if (!left.pre.length || !right.pre.length) return left.pre.length === right.pre.length ? 0 : left.pre.length ? -1 : 1;
  for (let i = 0; i < Math.max(left.pre.length, right.pre.length); i++) {
    const x = left.pre[i], y = right.pre[i];
    if (x === undefined || y === undefined) return x === undefined ? -1 : 1;
    if (x === y) continue;
    const nx = /^[0-9]+$/.test(x), ny = /^[0-9]+$/.test(y);
    if (nx && ny) return numeric(x, y);
    if (nx !== ny) return nx ? -1 : 1;
    return x < y ? -1 : 1;
  }
  return 0;
}
export function catalogState(installed: RegisteredModule | undefined, candidate: RegisteredModule["manifest"]) {
  if (!installed) return "available";
  if (installed.status === "registered") return "incomplete";
  const order = compareSemver(candidate.version, installed.manifest.version);
  if (order === null) return "invalid";
  if (order < 0) return "older";
  if (order > 0) return "newer";
  return installed.manifest.runtime.docker.image === candidate.runtime.docker.image ? "installed" : "changed";
}
export function runtimeMessage(status?: ModuleRuntime): UIKey {
  if (!status) return "runtimeUnknown";
  if (!status.available) return "runtimeAbsent";
  if (!status.active) return "runtimeInactive";
  if (!status.engine_ready) return "engineUnavailable";
  if (!status.network_ready) return "networkDegraded";
  return "runtimeReady";
}
export function activeJob(job: Job) { return ["queued", "running", "cancel_requested"].includes(job.status); }
export function canCancelJob(job: Job) {
  return job.status === "queued" || job.status === "running" && ["module.install", "module.control", "module.remove", "module.gpu.prepare"].includes(job.type);
}
export function jobTitle(job: Job): UIKey {
  return ({"module.gpu.prepare": "moduleGPUPrepareJob", "module.install": "moduleInstallJob", "module.upgrade": "moduleUpgradeJob", "module.control": "moduleControlJob", "module.remove": "moduleRemoveJob", "core.update.install": "updateJob"} as Record<string, UIKey>)[job.type] || "backgroundJob";
}
export function jobMessage(job: Job): UIKey {
  const known: Record<string, UIKey> = {
    "validating module manifest": "validating", "pulling and creating module container": "creating",
    "module container is running": "running", "applying module runtime operation": "applying",
    "module runtime state updated": "updated", "removing module container": "removing",
    "module removed; persistent data preserved": "removed",
  };
  if (["queued", "succeeded", "failed", "cancelled", "cancel_requested"].includes(job.status)) return job.status as UIKey;
  return known[job.message || ""] || "started";
}
export function progressPercent(job: Job) { return Number.isFinite(job.progress) ? Math.max(0, Math.min(100, job.progress / 100)) : 0; }
