import {describe, expect, it, vi} from "vitest";
import {renderToStaticMarkup} from "react-dom/server";
import type {Actor, ModuleNavigationState} from "../api/types";
import {accessiblePath, visibleNavigation} from "../navigation";
import {ModulePage} from "./Module";
vi.mock("../i18n", () => ({useI18n: () => ({t: (key: string) => key, date: (value: string) => value, status: (value: string) => value})}));
vi.mock("../ui/useUI", () => ({useUI: () => (key: string) => key}));
const manager: Actor = {type: "user", id: "owner", username: "owner", roles: ["owner"], permissions: ["security.self.read", "modules.read", "modules.manage"]};
function paths(modules: ModuleNavigationState[], actor = manager) {return visibleNavigation(actor, modules).flatMap(group => group.items.map(item => item.path));}
describe("installed-module sidebar contract", () => {
  it("adds the entry on registry refresh and keeps it across stop, restart and reload", () => {
    expect(paths([])).not.toContain("/modules/frigate.module");
    for (const status of ["enabled", "disabled", "error", "enabled"] as const) {
      const modules = [{module_id: "frigate.module", name: "Frigate", status}];
      expect(paths(modules)).toContain("/modules/frigate.module");
      expect(paths(JSON.parse(JSON.stringify(modules)))).toContain("/modules/frigate.module");
      expect(accessiblePath(manager, "/modules/frigate.module", modules)).toBe("/modules/frigate.module");
    }
    expect(paths([])).not.toContain("/modules/frigate.module");
  });
  it("supports old manifests with empty navigation for every installed module", () => {
    for (const id of ["homeassistant.module", "nvr.module", "ai.agent", "reference.module"]) {
      const modules: ModuleNavigationState[] = [{module_id: id, name: id, status: "enabled", items: []}];
      expect(paths(modules)).toContain("/modules/" + id);
    }
  });
  it("deduplicates declared entries and rejects links outside the module", () => {
    const modules: ModuleNavigationState[] = [{module_id: "frigate.module", status: "enabled", items: [
      {id: "one", title: "Frigate", route: "/modules/frigate.module"},
      {id: "two", title: "Duplicate", route: "/modules/frigate.module"},
      {id: "foreign", title: "Wrong", route: "/modules/ai.agent"},
    ]}];
    expect(paths(modules).filter(path => path === "/modules/frigate.module")).toHaveLength(1);
    expect(paths(modules)).not.toContain("/modules/ai.agent");
  });
  it("does not expose Frigate to a read-only user or infer access from owner role", () => {
    const modules: ModuleNavigationState[] = [{module_id: "frigate.module", status: "enabled"}];
    const reader = {...manager, permissions: ["security.self.read", "modules.read"]};
    expect(paths(modules, reader)).not.toContain("/modules/frigate.module");
    expect(accessiblePath(reader, "/modules/frigate.module", modules)).toBe("/");
  });
  it("opens stopped modules on a working management page, not an unavailable-route placeholder", () => {
    const html = renderToStaticMarkup(<ModulePage path="/modules/frigate.module" modules={[{module_id: "frigate.module", name: "Frigate", status: "disabled"}]} permitted canReadModules canManage onNavigate={() => {}} />);
    expect(html).toContain("Frigate");
    expect(html).toContain("Запустить");
    expect(html).not.toContain("routeUnavailable");
  });
  it("never renders module controls when the route is not permitted", () => {
    const html = renderToStaticMarkup(<ModulePage path="/modules/frigate.module" modules={[{module_id: "frigate.module", status: "disabled"}]} permitted={false} canReadModules={false} canManage={false} onNavigate={() => {}} />);
    expect(html).not.toContain("Запустить");
    expect(html).toContain("unknownRoute");
  });
});
