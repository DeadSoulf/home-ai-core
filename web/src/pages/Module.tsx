import {useState} from "react";
import type {ModuleNavigationState} from "../api/types";
import {api} from "../api/client";
import {Panel} from "../components/Panel";
import {ErrorNotice} from "../components/ErrorNotice";
import {PageHeading, Status} from "./Dashboard";
import {useUI} from "../ui/useUI";
import {homeAssistantURL} from "../ui/homeAssistant";

// Every installed module has a usable management page, including legacy
// manifests and stopped modules. Native active-module pages are selected by App.
export function ModulePage({path, modules, permitted, canReadModules, canManage = false, onNavigate}: {
  path: string; modules: ModuleNavigationState[]; permitted: boolean; canReadModules: boolean;
  canManage?: boolean; onNavigate: (path: string) => void;
}) {
  const u = useUI();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const module = permitted ? modules.find(item => path === "/modules/" + item.module_id || item.items?.some(nav => nav.route === path)) : undefined;
  const item = module?.items?.find(entry => entry.route === path);
  async function control(operation: "enable" | "disable" | "restart") {
    if (!module || !canManage || busy) return;
    setBusy(true); setError(undefined);
    try {
      await api.controlModule(module.module_id, operation);
      window.dispatchEvent(new CustomEvent("home-ai-core:modules-changed"));
      onNavigate("/modules/operations");
    } catch (reason) {setError(reason);} finally {setBusy(false);}
  }
  return <div className="page">
    <PageHeading title={item?.title || module?.name || module?.module_id || u("unknownRoute")} subtitle={path} />
    {error !== undefined && <ErrorNotice error={error} />}
    {module ? <Panel title="Управление модулем">
      <p><Status value={module.status} /></p>
      <p className="mono small">{module.module_id}</p>
      {module.status === "disabled" && <p>Модуль установлен и остановлен. Данные сохранены; для работы запустите его.</p>}
      {module.status === "error" && <p>Модуль установлен, но сообщил об ошибке. Проверьте состояние и последние операции.</p>}
      {module.status === "registered" && <p>Установка ещё не завершена. Откройте список модулей, чтобы проверить задачу или повторить установку.</p>}
      {module.module_id === "homeassistant.module" && module.status === "enabled" && typeof window !== "undefined" &&
        <p><a className="button primary" href={homeAssistantURL(window.location.href)} target="_blank" rel="noopener noreferrer">Открыть Home Assistant</a></p>}
      {canManage && module.status !== "registered" && <div className="button-row">
        <button type="button" className="button primary" disabled={busy} onClick={() => void control(module.status === "enabled" ? "disable" : "enable")}>{module.status === "enabled" ? "Остановить" : "Запустить"}</button>
        <button type="button" className="button secondary" disabled={busy} onClick={() => void control("restart")}>Перезапустить</button>
      </div>}
      {canReadModules && <p><button type="button" className="button secondary" onClick={() => onNavigate("/modules/installed")}>Состояние, обновления и удаление</button></p>}
    </Panel> : <Panel title={u("unknownRoute")}><p role="status">{u("unknownRouteHint")}</p>
      {canReadModules && <button type="button" className="button secondary" onClick={() => onNavigate("/modules")}>{u("backModules")}</button>}
    </Panel>}
  </div>;
}
