import type { ModuleNavigationState } from "../api/types";
import { Panel } from "../components/Panel";
import { PageHeading } from "./Dashboard";
import { useUI } from "../ui/useUI";
export function ModulePage({path, modules, permitted, canReadModules, onNavigate}: {
  path: string; modules: ModuleNavigationState[]; permitted: boolean; canReadModules: boolean; onNavigate: (path: string) => void;
}) {
  const u = useUI();
  const item = permitted ? modules.filter((module) => module.status === "enabled").flatMap((module) => module.items || []).find((item) => item.route === path) : undefined;
  return <div className="page"><PageHeading title={item?.title || u("unknownRoute")} subtitle={path} />
    <Panel title={u(item ? "routeUnavailable" : "unknownRoute")}><p role="status">{u(item ? "routeHint" : "unknownRouteHint")}</p>
      {canReadModules && <button type="button" className="button secondary" onClick={() => onNavigate("/modules")}>{u("backModules")}</button>}
    </Panel>
  </div>;
}
