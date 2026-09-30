import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Actor } from "./api/types";
import { Shell } from "./components/Shell";
import { I18nProvider } from "./i18n";
import { accessiblePath, hasPermission, systemSection, visibleNavigation } from "./navigation";

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
    expect(nav.indexOf("Server")).toBeGreaterThan(-1);
    expect(nav.indexOf("Services")).toBeGreaterThan(nav.indexOf("Server"));
    expect(nav.indexOf("Management")).toBeGreaterThan(nav.indexOf("Services"));
    expect(nav).toContain("Home");
    expect(nav).toContain("System");
    expect(nav).toContain("Operations");
    expect(nav).toContain("Security");
    expect(nav.match(/aria-current="page"/g)).toHaveLength(1);
    expect(nav).toMatch(/<button\b[^>]*aria-current="page"[^>]*><span>Security<\/span><\/button>/);
  });

  it("omits inaccessible entries and their empty group headings", () => {
    const nav = renderNavigation(actor(["jobs.read"]), "/jobs");
    expect(nav).toContain("Operations");
    expect(nav).toContain("Management");
    expect(nav).toContain("Home");
    for (const label of ["Server", "Services", "System", "Updates", "Modules", "Security"]) {
      expect(nav).not.toContain(label);
    }
    expect(nav.match(/aria-current="page"/g)).toHaveLength(1);
  });
});
