import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Actor } from "./api/types";
import { Shell } from "./components/Shell";
import { I18nProvider } from "./i18n";
import { accessiblePath, fileSection, fileSectionPath, hasPermission, isModuleRoute, isModuleManagementRoute, moduleSection, moduleSectionPath, systemSection, visibleNavigation } from "./navigation";

function actor(permissions: string[], roles = ["owner"]): Actor {
  return {type: "user", id: "usr-test", username: "reader", roles, permissions};
}

const allReads = ["system.read", "modules.read", "updates.read", "jobs.read", "audit.read", "security.self.read", "security.users.read"];

describe("navigation access", () => {
  it("preserves System deep links and requires read access for updates", () => {
    const reader = actor(allReads);
    for (const section of ["equipment", "storage", "network", "updates"]) {
      expect(accessiblePath(reader, "/system#" + section)).toBe("/system#" + section);
      expect(systemSection("/system#" + section)).toBe(section);
    }
    expect(accessiblePath(actor(["system.read"]), "/system#updates")).toBe("/system");
    expect(accessiblePath(actor(["updates.read"]), "/system#updates")).toBe("/");
    expect(accessiblePath(reader, "/system#unknown")).toBe("/system#equipment");
  });
  it("uses stable Files routes and keeps legacy hash links compatible", () => {
    const manager = actor([...allReads, "files.manage"]);
    expect(fileSectionPath("folders")).toBe("/files");
    expect(fileSectionPath("storage")).toBe("/files/storage");
    expect(fileSectionPath("windows")).toBe("/files/windows");

    for (const section of ["folders", "storage", "windows"] as const) {
      const path = fileSectionPath(section);
      expect(accessiblePath(manager, path)).toBe(path);
      expect(fileSection(path)).toBe(section);
      expect(accessiblePath(manager, "/files#" + section)).toBe(path);
    }

    expect(accessiblePath(actor(["security.self.read"]), "/files/storage")).toBe("/files");
    expect(accessiblePath(actor(["security.self.read"]), "/files#storage")).toBe("/files");
    expect(accessiblePath(actor(["files.manage"]), "/files/storage")).toBe("/");
    expect(accessiblePath(manager, "/files/unknown")).toBe("/files");
    expect(accessiblePath(manager, "/files#unknown")).toBe("/files");
  });

  it("uses separate, protected module-management pages without intercepting extension routes", () => {
    const manager = actor(allReads);
    for (const section of ["installed", "catalog", "operations", "settings"] as const) {
      const route = moduleSectionPath(section);
      expect(moduleSection(route)).toBe(section);
      expect(isModuleManagementRoute(route)).toBe(true);
      expect(isModuleRoute(route)).toBe(false);
      expect(accessiblePath(manager, route)).toBe(route);
      expect(accessiblePath(actor(["security.self.read"]), route)).toBe("/");
    }
    expect(moduleSection("/modules")).toBe("installed");
    expect(accessiblePath(manager, "/modules")).toBe("/modules");
    expect(isModuleRoute("/modules/nvr.module")).toBe(true);
    expect(isModuleManagementRoute("/modules/nvr.module")).toBe(false);
    expect(isModuleRoute("/modules/catalog")).toBe(false);
  });

  it("shows AI Agent only to module managers while other modules retain access", () => {
    const moduleItems = [
      {module_id: "ai.agent", status: "enabled" as const, items: [{id:"ai.agent", title:"AI Agent", route:"/modules/ai.agent"}]},
      {module_id: "nvr.module", status: "enabled" as const, items: [{id:"nvr.module", title:"NVR", route:"/modules/nvr.module"}]},
    ];
    const reader = actor(["security.self.read", "modules.read"]);
    const manager = actor(["security.self.read", "modules.read", "modules.manage"]);
    const visibleToReader = visibleNavigation(reader, moduleItems).flatMap((g) => g.items.map((x) => x.path));
    expect(visibleToReader).not.toContain("/modules/ai.agent");
    expect(visibleToReader).toContain("/modules/nvr.module");
    expect(accessiblePath(reader, "/modules/ai.agent", moduleItems)).toBe("/");
    expect(accessiblePath(manager, "/modules/ai.agent", moduleItems)).toBe("/modules/ai.agent");
  });

  it("keeps existing URLs accessible for an actor with the corresponding read permissions", () => {
    const reader = actor(allReads);
    for (const path of ["/", "/system", "/modules", "/files", "/jobs", "/audit", "/users"]) {
      expect(accessiblePath(reader, path)).toBe(path);
    }
    expect(accessiblePath(reader, "/system/")).toBe("/system");
    expect(accessiblePath(reader, "/unknown")).toBe("/");
  });

  it("keeps Home available and removes empty groups for a limited reader", () => {
    const reader = actor(["jobs.read", "audit.read"]);
    const groups = visibleNavigation(reader);
    expect(groups.map((group) => group.label)).toEqual(["home", "managementGroup"]);
    expect(groups.flatMap((group) => group.items.map((item) => item.path))).toEqual([
      "/", "/jobs", "/audit",
    ]);
    for (const path of ["/system", "/modules", "/updates"]) {
      expect(accessiblePath(reader, path)).toBe("/");
    }
  });

  it("does not infer access from owner role or a similar permission name", () => {
    const restrictedOwner = actor(["system.read.extra", "updates.manage"]);
    expect(hasPermission(restrictedOwner, "system.read")).toBe(false);
    expect(hasPermission(restrictedOwner, "updates.read")).toBe(false);
    expect(accessiblePath(restrictedOwner, "/system")).toBe("/");
    expect(accessiblePath(restrictedOwner, "/updates")).toBe("/");
    expect(visibleNavigation(restrictedOwner).flatMap((group) => group.items.map((item) => item.path))).toEqual(["/"]);
  });
});

describe("navigation rendering", () => {
  beforeEach(() => {
    vi.stubGlobal("navigator", {languages: ["en-US"]});
    vi.stubGlobal("localStorage", {getItem: () => "en", setItem: vi.fn(), removeItem: vi.fn()});
  });

  afterEach(() => vi.unstubAllGlobals());

  function renderNavigation(reader: Actor, path: string): string {
    const markup = renderToStaticMarkup(
      <I18nProvider>
        <Shell
          actor={reader}
          path={path}
          realtime="disconnected"
          onNavigate={() => undefined}
          onLogout={() => undefined}
        >
          <p>Page content</p>
        </Shell>
      </I18nProvider>,
    );
    const nav = markup.match(/<nav\b[^>]*>[\s\S]*?<\/nav>/)?.[0];
    expect(nav).toBeDefined();
    return nav!;
  }

  it("renders task groups in order and exposes one active page to assistive technology", () => {
    const nav = renderNavigation(actor(allReads), "/audit");
    const headings = [...nav.matchAll(/class="nav-group-label">([^<]+)<\/span>/g)].map((match) => match[1]);
    expect(headings).toEqual(["Services", "Server", "Management"]);
    expect(nav).toContain("Home");
    expect(nav).toContain("System");
    expect(nav).toContain("Operations");
    expect(nav).toContain("Security");
    expect(nav.match(/aria-current="page"/g)).toHaveLength(1);
    expect(nav).toMatch(/<button\b[^>]*aria-current="page"[^>]*>[\s\S]*?<span>Security<\/span>/);
  });

  it("keeps Modules highlighted across its management pages", () => {
    for (const path of ["/modules/installed", "/modules/catalog", "/modules/operations", "/modules/settings"]) {
      const nav = renderNavigation(actor(allReads), path);
      expect(nav.match(/aria-current="page"/g)).toHaveLength(1);
    expect(nav).toMatch(/<button\b[^>]*aria-current="page"[^>]*>[\s\S]*?<span>Modules<\/span>/);
    }
  });

  it("keeps Files highlighted on nested section routes", () => {
    const nav = renderNavigation(actor([...allReads, "files.manage"]), "/files/storage");
    expect(nav.match(/aria-current="page"/g)).toHaveLength(1);
    expect(nav).toMatch(/<button\b[^>]*aria-current="page"[^>]*>[\s\S]*?<span>Files<\/span>/);
  });

  it("omits inaccessible entries and their empty group headings", () => {
    const nav = renderNavigation(actor(["jobs.read"]), "/jobs");
    expect(nav).toContain("Operations");
    expect(nav).toContain("Management");
    expect(nav).toContain("Home");
    for (const label of ["Server", "Services"]) {
      expect(nav).not.toContain('<span class="nav-group-label">' + label + "</span>");
    }
    for (const label of ["System", "Updates", "Modules", "Security"]) {
      expect(nav).not.toContain("<span>" + label + "</span>");
    }
    expect(nav.match(/aria-current="page"/g)).toHaveLength(1);
  });
});
