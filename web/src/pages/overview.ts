import { api } from "../api/client";
import type { Actor } from "../api/types";
import { hasPermission } from "../navigation";

type Section = "system" | "modules" | "jobs";
type OverviewClient = Pick<typeof api, Section>;

export async function loadOverview(actor: Actor, client: OverviewClient = api) {
  const results = await Promise.allSettled([
    hasPermission(actor, "system.read") ? client.system() : Promise.resolve(undefined),
    hasPermission(actor, "modules.read") ? client.modules() : Promise.resolve(undefined),
    hasPermission(actor, "jobs.read") ? client.jobs() : Promise.resolve(undefined),
  ]);
  const errors: { section: Section; message: string }[] = [];
  function read<T>(section: Section, result: PromiseSettledResult<T>): T | undefined {
    if (result.status === "fulfilled") return result.value;
    errors.push({ section, message: result.reason instanceof Error ? result.reason.message : String(result.reason) });
    return undefined;
  }
  return {
    system: read("system", results[0]),
    modules: read("modules", results[1]),
    jobs: read("jobs", results[2]),
    errors,
  };
}
