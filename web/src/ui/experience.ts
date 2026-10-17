import type { UIKey } from "./messages";

export type QuickDestination = {
  path: string;
  title: UIKey;
  description: UIKey;
  permissions: readonly string[];
};

export const quickDestinations: readonly QuickDestination[] = [
  {path: "/files", title: "uxQuickFiles", description: "uxQuickFilesDescription", permissions: ["security.self.read"]},
  {path: "/modules", title: "uxQuickModules", description: "uxQuickModulesDescription", permissions: ["modules.read"]},
  {path: "/system#storage", title: "uxQuickStorage", description: "uxQuickStorageDescription", permissions: ["system.read"]},
  {path: "/system#network", title: "uxQuickNetwork", description: "uxQuickNetworkDescription", permissions: ["system.read", "network.read"]},
  {path: "/users", title: "uxQuickUsers", description: "uxQuickUsersDescription", permissions: ["security.users.read"]},
  {path: "/jobs", title: "uxQuickOperations", description: "uxQuickOperationsDescription", permissions: ["jobs.read"]},
  {path: "/system#updates", title: "uxQuickUpdates", description: "uxQuickUpdatesDescription", permissions: ["system.read", "updates.read"]},
];

export function visibleQuickDestinations(permissions: readonly string[]): QuickDestination[] {
  const allowed = new Set(permissions);
  return quickDestinations.filter((item) => item.permissions.every((permission) => allowed.has(permission)));
}

export function navigationHint(path: string): UIKey {
  switch (path) {
    case "/": return "uxNavHome";
    case "/files": return "uxNavFiles";
    case "/modules": return "uxNavModules";
    case "/system": return "uxNavSystem";
    case "/users": return "uxNavUsers";
    case "/jobs": return "uxNavJobs";
    case "/audit": return "uxNavAudit";
    default: return "uxNavModulePage";
  }
}

export type SystemGuide = { title: UIKey; description: UIKey };
export const systemGuides = {
  equipment: {title: "uxSystemEquipment", description: "uxSystemEquipmentDescription"},
  storage: {title: "uxSystemStorage", description: "uxSystemStorageDescription"},
  network: {title: "uxSystemNetwork", description: "uxSystemNetworkDescription"},
  updates: {title: "uxSystemUpdates", description: "uxSystemUpdatesDescription"},
  console: {title: "uxSystemConsole", description: "uxSystemConsoleDescription"},
} as const satisfies Record<string, SystemGuide>;

export const filesGuides = {
  folders: {title: "uxFilesFolders", description: "uxFilesFoldersDescription"},
  storage: {title: "uxFilesStorage", description: "uxFilesStorageDescription"},
  windows: {title: "uxFilesWindows", description: "uxFilesWindowsDescription"},
} as const satisfies Record<string, SystemGuide>;
