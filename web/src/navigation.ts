import type { Actor } from "./api/types";
import type { StringKey } from "./i18n";

type NavigationItem = { path: string; label: StringKey; permission?: string };
type NavigationGroup = { label: StringKey; items: NavigationItem[] };

export const navigationGroups: NavigationGroup[] = [
  { label: "home", items: [{ path: "/", label: "home" }] },
  { label: "serverGroup", items: [
    { path: "/system", label: "system", permission: "system.read" },
  ] },
  { label: "serviceGroup", items: [
    { path: "/files", label: "files", permission: "security.self.read" },
    { path: "/modules", label: "modules", permission: "modules.read" },
  ] },
  { label: "managementGroup", items: [
    { path: "/users", label: "users", permission: "security.users.read" },
    { path: "/jobs", label: "activity", permission: "jobs.read" },
    { path: "/audit", label: "securityLog", permission: "audit.read" },
  ] },
];

export function hasPermission(actor: Actor, permission: string): boolean {
  return actor.permissions.includes(permission);
}

export function visibleNavigation(actor: Actor): NavigationGroup[] {
  return navigationGroups.map((group) => ({
    ...group,
    items: group.items.filter((item) => !item.permission || hasPermission(actor, item.permission)),
  })).filter((group) => group.items.length > 0);
}

export const systemSections = ["equipment", "storage", "network", "updates"] as const;
export type SystemSection = typeof systemSections[number];

export function systemSection(path: string): SystemSection {
  const value = path.split("#")[1];
  return systemSections.find((section) => section === value) || "equipment";
}

export function accessiblePath(actor: Actor, path: string): string {
  const [raw, hash] = path.split("#");
  const route = raw.replace(/\/+$/, "") || "/";
  const allowed = visibleNavigation(actor).some((group) => group.items.some((item) => item.path === route));
  if (!allowed) return "/";
  if (route !== "/system" || !hash) return route;
  const section = systemSection(path);
  if (section === "updates" && !hasPermission(actor, "updates.read")) return route;
  return route + "#" + section;
}
