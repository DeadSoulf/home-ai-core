import { describe, expect, it, vi } from "vitest";
import type { Actor, Job, RegisteredModule, SystemResponse } from "../api/types";
import { loadOverview } from "./overview";

function actor(permissions: string[]): Actor {
  return {type: "user", id: "usr-test", roles: ["owner"], permissions};
}

const system: SystemResponse = {
  version: "test-version",
  schema_version: 6,
  system: {
    node_id: "node-test", hostname: "home-server", os: "linux", architecture: "amd64",
    cpu: {logical_cpus: 4, usage_percent: 0}, memory: {total_bytes: 8 * 1024 ** 3},
    block_tree: [], network_interfaces: [], gpus: [],
  },
};
const modules: RegisteredModule[] = [{
  manifest: {schema_version: 1, id: "demo", name: "Demo", version: "1.0.0", core: ">=0.1.0", lifecycle: []},
  status: "registered",
}];
const jobs: Job[] = [{
  id: "job-test", node_id: "node-test", type: "core.update.install", status: "running",
  progress: 500, created_at: "2026-09-30T00:00:00Z",
}];

function client() {
  return {
    system: vi.fn(async () => system),
    modules: vi.fn(async () => modules),
    jobs: vi.fn(async () => jobs),
  };
}

describe("permission-aware overview loading", () => {
  it("loads equipment alone without making forbidden service/activity requests", async () => {
    const api = client();
    const result = await loadOverview(actor(["system.read"]), api);
    expect(api.system).toHaveBeenCalledTimes(1);
    expect(api.modules).not.toHaveBeenCalled();
    expect(api.jobs).not.toHaveBeenCalled();
    expect(result.system).toBe(system);
    expect(result.modules).toBeUndefined();
    expect(result.jobs).toBeUndefined();
    expect(result.errors).toEqual([]);
  });

  it("works for an activity-only reader without assuming system.read", async () => {
    const api = client();
    const result = await loadOverview(actor(["jobs.read"]), api);
    expect(api.jobs).toHaveBeenCalledTimes(1);
    expect(api.system).not.toHaveBeenCalled();
    expect(api.modules).not.toHaveBeenCalled();
    expect(result.jobs).toBe(jobs);
    expect(result.system).toBeUndefined();
    expect(result.modules).toBeUndefined();
    expect(result.errors).toEqual([]);
  });

  it("keeps successful sections when one permitted API fails and does not invent an empty result", async () => {
    const api = client();
    api.modules.mockRejectedValueOnce(new Error("service list unavailable"));
    const result = await loadOverview(actor(["system.read", "modules.read", "jobs.read"]), api);
    expect(result.system).toBe(system);
    expect(result.jobs).toBe(jobs);
    expect(result.modules).toBeUndefined();
    expect(result.errors).toEqual([{section: "modules", message: "service list unavailable"}]);
    expect(api.system).toHaveBeenCalledTimes(1);
    expect(api.modules).toHaveBeenCalledTimes(1);
    expect(api.jobs).toHaveBeenCalledTimes(1);
  });

  it("distinguishes a permitted empty list from data the actor cannot read", async () => {
    const api = client();
    api.modules.mockResolvedValueOnce([]);
    const result = await loadOverview(actor(["modules.read"]), api);
    expect(result.modules).toEqual([]);
    expect(result.jobs).toBeUndefined();
    expect(result.system).toBeUndefined();
    expect(result.errors).toEqual([]);
  });

  it("makes no requests and provides no fake zero-state data without read rights", async () => {
    const api = client();
    const result = await loadOverview(actor([]), api);
    expect(api.system).not.toHaveBeenCalled();
    expect(api.modules).not.toHaveBeenCalled();
    expect(api.jobs).not.toHaveBeenCalled();
    expect(result.system).toBeUndefined();
    expect(result.modules).toBeUndefined();
    expect(result.jobs).toBeUndefined();
    expect(result.errors).toEqual([]);
  });

  it("reports all permitted section failures without rejecting the complete overview", async () => {
    const api = client();
    api.system.mockRejectedValueOnce(new Error("equipment unavailable"));
    api.modules.mockRejectedValueOnce("services unavailable");
    api.jobs.mockRejectedValueOnce(new Error("activity unavailable"));
    const result = await loadOverview(actor(["system.read", "modules.read", "jobs.read"]), api);
    expect(result.system).toBeUndefined();
    expect(result.modules).toBeUndefined();
    expect(result.jobs).toBeUndefined();
    expect(result.errors).toEqual([
      {section: "system", message: "equipment unavailable"},
      {section: "modules", message: "services unavailable"},
      {section: "jobs", message: "activity unavailable"},
    ]);
  });
});
