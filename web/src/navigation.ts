import type { Actor, ModuleNavigationState } from "./api/types";
import type { StringKey } from "./i18n";

export type NavigationItem = {
  path: string;
  label?: StringKey;
  title?: string;
  permission?: string;
  moduleId?: string;
  order?: number;
};

type NavigationGroup = { label: StringKey; items: NavigationItem[] };

export const navigationGroups: NavigationGroup[] = [
  { label: "home", items: [{ path: "/", label: "home" }] },
  { label: "serverGroup", items: [
    { path: "/system", label: "system", permission: "system.read" },
  ] },
  { label: "serviceGroup", items: [
    { path: "/ai", label: "aiAgent", permission: "security.self.read", moduleId: "ai.agent" },
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

function moduleIsEnabled(moduleId: string, modules?: ModuleNavigationState[]): boolean {
  if (!modules) return true;
  return modules.some((item) => item.module_id === moduleId && item.status === "enabled");
}

function dynamicModuleItems(modules?: ModuleNavigationState[]): NavigationItem[] {
  if (!modules) return [];
  return modules
    .filter((module) => module.status === "enabled")
    .flatMap((module) => (module.items || []).map((item) => ({
      path: item.route,
      label: module.module_id === "ai.cloud"
        ? ("cloudAI" as StringKey)
        : module.module_id === "cameras" && item.route === "/modules/cameras"
          ? ("cameras" as StringKey)
          : undefined,
      title: item.title,
      permission: "security.self.read",
      moduleId: module.module_id,
      order: item.order || 0,
    })))
    .sort((a, b) => (a.order || 0) - (b.order || 0) || a.path.localeCompare(b.path));
}

export function visibleNavigation(actor: Actor, modules?: ModuleNavigationState[]): NavigationGroup[] {
  const dynamic = dynamicModuleItems(modules);
  return navigationGroups.map((group) => {
    let items = group.items.filter((item) =>
      (!item.permission || hasPermission(actor, item.permission)) &&
      (!item.moduleId || moduleIsEnabled(item.moduleId, modules)),
    );
    if (group.label === "serviceGroup") {
      items = [
        ...items,
        ...dynamic.filter((item) => !item.permission || hasPermission(actor, item.permission)),
      ];
    }
    return {...group, items};
  }).filter((group) => group.items.length > 0);
}

export const systemSections = ["equipment", "storage", "network", "updates"] as const;
export type SystemSection = typeof systemSections[number];

export const fileSections = ["folders", "storage", "windows"] as const;
export type FileSection = typeof fileSections[number];

export function fileSection(path: string): FileSection {
  const [raw, hash] = path.split("#");
  const route = raw.replace(/\/+$/, "") || "/";
  const nested = route.startsWith("/files/") ? route.slice("/files/".length) : "";
  const value = nested || hash;
  return fileSections.find((section) => section === value) || "folders";
}

export function fileSectionPath(section: FileSection): string {
  return section === "folders" ? "/files" : "/files/" + section;
}

export function systemSection(path: string): SystemSection {
  const value = path.split("#")[1];
  return systemSections.find((section) => section === value) || "equipment";
}

export function accessiblePath(actor: Actor, path: string, modules?: ModuleNavigationState[]): string {
  const [raw, hash] = path.split("#");
  const route = raw.replace(/\/+$/, "") || "/";
  const fileRoute = route === "/files" || route.startsWith("/files/");
  const navigationRoute = fileRoute ? "/files" : route;
  const allowed = navigationRoute === "/account" ||
    visibleNavigation(actor, modules).some((group) => group.items.some((item) => item.path === navigationRoute));
  if (!allowed) return "/";

  if (fileRoute) {
    const section = fileSection(path);
    if (section !== "folders" && !hasPermission(actor, "files.manage")) return "/files";
    return fileSectionPath(section);
  }

  if (!hash) return route;
  if (route === "/system") {
    const section = systemSection(path);
    if (section === "updates" && !hasPermission(actor, "updates.read")) return route;
    return route + "#" + section;
  }
  return route;
}
