import type { Actor, ModuleNavigationState } from "./api/types";
import type { StringKey } from "./i18n";

export type NavigationItem = {
  path: string;
  label?: StringKey;
  title?: string;
  permission?: string;
  moduleId?: string;
  moduleStatus?: ModuleNavigationState["status"];
  order?: number;
};

type NavigationGroup = { label: StringKey; items: NavigationItem[] };

export const navigationGroups: NavigationGroup[] = [
  { label: "home", items: [{ path: "/", label: "home" }] },
  // Everyday services appear before advanced server administration.
  { label: "serviceGroup", items: [
    { path: "/files", label: "files", permission: "security.self.read" },
    { path: "/modules", label: "modules", permission: "modules.read" },
  ] },
  { label: "serverGroup", items: [
    { path: "/system", label: "system", permission: "system.read" },
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

export const moduleSections = ["installed", "catalog", "operations", "settings"] as const;
export type ModuleSection = typeof moduleSections[number];

export function moduleSectionPath(section: ModuleSection): string {
  return "/modules/" + section;
}

export function isModuleManagementRoute(path: string): boolean {
  return path === "/modules" || moduleSections.some((section) => path === moduleSectionPath(section));
}

export function moduleSection(path: string): ModuleSection {
  const route = path.split("#")[0].replace(/\/+$/, "") || "/";
  return moduleSections.find((section) => route === moduleSectionPath(section)) || "installed";
}

export function isModuleRoute(path: string): boolean {
  return !isModuleManagementRoute(path) &&
    /^\/modules\/[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*(?:\/[A-Za-z0-9._~-]+)*$/.test(path) &&
    !path.split("/").some((part) => part === "." || part === "..");
}

function dynamicModuleItems(modules?: ModuleNavigationState[]): NavigationItem[] {
  const entries = new Map<string, NavigationItem>();
  for (const module of modules || []) {
    const root = "/modules/" + module.module_id;
    const declared = (module.items || []).filter(item => isModuleRoute(item.route) && (item.route === root || item.route.startsWith(root + "/")));
    if (!declared.some(item => item.route === root)) declared.push({id: module.module_id, title: module.name || module.module_id, route: root});
    for (const item of declared) {
      if (!isModuleRoute(item.route) || entries.has(item.route)) continue;
      entries.set(item.route, {
        path: item.route, title: item.title, moduleId: module.module_id, moduleStatus: module.status,
        permission: module.module_id === "ai.agent" || module.module_id === "frigate.module" ? "modules.manage" : "modules.read",
        order: item.order || 0,
      });
    }
  }
  return [...entries.values()].sort((a, b) => (a.order || 0) - (b.order || 0) || a.path.localeCompare(b.path));
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

export const systemSections = ["equipment", "storage", "network", "updates", "console"] as const;
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
  const navigationRoute = fileRoute ? "/files" : isModuleManagementRoute(route) ? "/modules" : route;
  const allowed = navigationRoute === "/account" ||
    visibleNavigation(actor, modules).some((group) => group.items.some((item) => item.path === navigationRoute));
  if (!allowed) return "/";

  // Management sections are first-party pages, never routes owned by a Docker module.
  if (isModuleManagementRoute(route)) return route;

  if (fileRoute) {
    const section = fileSection(path);
    if (section !== "folders" && !hasPermission(actor, "files.manage")) return "/files";
    return fileSectionPath(section);
  }

  if (!hash) return route;
  if (route === "/jobs" && /^job=[a-zA-Z0-9_-]{1,128}$/.test(hash)) return route + "#" + hash;
  if (route === "/system") {
    const section = systemSection(path);
    if (section === "updates" && !hasPermission(actor, "updates.read")) return route;
    if (section === "console" && !hasPermission(actor, "system.console")) return route;
    return route + "#" + section;
  }
  return route;
}
