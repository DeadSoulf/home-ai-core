import {useCallback, useEffect, useState} from "react";
import {api} from "../api/client";
import type {FrigateConfig, FrigateEvent, FrigateStats} from "../api/types";
import {useResource} from "../hooks/useResource";
import {ErrorNotice} from "../components/ErrorNotice";
import {LoadingState, Panel} from "../components/Panel";
import {ConfirmDialog} from "../components/ConfirmDialog";
import {PageHeading} from "./Dashboard";

// This page deliberately renders data, not JavaScript or HTML from the module.
// Camera credentials stay in the admin-only editor and are never in image URLs.
export function FrigatePage({onNavigate}: {onNavigate: (path: string) => void}) {
  const load = useCallback((signal: AbortSignal) => Promise.all([
    api.frigateVersion(signal), api.frigateConfig(signal), api.frigateStats(signal), api.frigateEvents(signal),
  ]).then(([version, config, stats, events]) => ({version: version.version, config, stats, events})), []);
  const data = useResource(load, 0, 15000);
  const [frame, setFrame] = useState(0);
  const [editing, setEditing] = useState(false);
  const [configText, setConfigText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [message, setMessage] = useState("");
  const [confirmSave, setConfirmSave] = useState(false);
  const [confirmRestart, setConfirmRestart] = useState(false);
  const [selected, setSelected] = useState<FrigateEvent>();
  useEffect(() => {const timer = window.setInterval(() => setFrame(value => value + 1), 5000); return () => window.clearInterval(timer);}, []);
  async function editConfig() {
    setBusy(true); setError(undefined); setMessage("");
    try {const result = await api.frigateRawConfig(); setConfigText(result.config); setEditing(true);}
    catch (reason) {setError(reason);} finally {setBusy(false);}
  }
  async function saveConfig() {
    setConfirmSave(false); setBusy(true); setError(undefined); setMessage("");
    try {
      const result = await api.frigateSaveConfig(configText);
      if (result.success === false) throw new Error(result.message || "Frigate отклонил конфигурацию");
      setMessage("Конфигурация проверена Frigate и сохранена. Для применения изменений перезапустите модуль.");
      setEditing(false); setConfigText("");
    } catch (reason) {setError(reason);} finally {setBusy(false);}
  }
  async function restart() {
    setConfirmRestart(false); setBusy(true); setError(undefined);
    try {await api.controlModule("frigate.module", "restart"); onNavigate("/modules/operations");}
    catch (reason) {setError(reason);} finally {setBusy(false);}
  }
  const cameras = Object.keys(data.data?.config.cameras || {});
  return <div className="page frigate-page">
    <PageHeading title="Frigate" subtitle={data.data ? `Официальный Frigate ${data.data.version}` : "Видеонаблюдение и события"} />
    <div className="button-row">
      <button type="button" className="button secondary" disabled={data.loading} onClick={data.reload}>Обновить данные</button>
      <button type="button" className="button secondary" onClick={() => onNavigate("/modules")}>Модули и обновления</button>
      <button type="button" className="button secondary" disabled={busy} onClick={() => void editConfig()}>Настроить камеры</button>
      <button type="button" className="button secondary" disabled={busy} onClick={() => setConfirmRestart(true)}>Перезапустить Frigate</button>
    </div>
    {data.loading && !data.data && <LoadingState />}
    {data.error && <ErrorNotice error={data.error} />}
    {error !== undefined && <ErrorNotice error={error} />}
    {message && <p className="notice" role="status">{message}</p>}
    <Panel title="Камеры">
      {cameras.length === 0 && <p>Камеры ещё не добавлены. Откройте «Настроить камеры» и укажите RTSP-потоки в конфигурации Frigate.</p>}
      <div className="card-grid">{cameras.map(name => <Camera key={name} name={name} config={data.data!.config} stats={data.data!.stats} frame={frame} />)}</div>
    </Panel>
    <Panel title="Последние события">
      {!data.data?.events.length && <p>Событий пока нет. Они появятся после подключения камер и включения детекции.</p>}
      <div className="card-grid">{data.data?.events.map(event => <article className="module-card" key={event.id}>
        <h3>{event.camera} — {event.label}</h3>
        <p>{new Date(event.start_time * 1000).toLocaleString()}</p>
        {event.has_snapshot && <img loading="lazy" alt={`Событие ${event.label}`} src={eventURL(event.id, "snapshot.jpg")} style={{width: "100%", objectFit: "contain"}} />}
        {event.has_clip && <button type="button" className="button secondary" onClick={() => setSelected(event)}>Просмотреть запись</button>}
      </article>)}</div>
    </Panel>
    {selected && <Panel title={`Запись: ${selected.camera}`} action={<button type="button" className="button secondary" onClick={() => setSelected(undefined)}>Закрыть</button>}>
      <video key={selected.id} controls preload="metadata" src={eventURL(selected.id, "clip.mp4")} style={{width: "100%", maxHeight: "65vh"}} />
    </Panel>}
    {editing && <Panel title="Конфигурация Frigate (YAML)">
      <p>Конфигурация может содержать пароли камер. Она доступна только администраторам модулей и не записывается в журнал аудита.</p>
      <p>Оставьте базу данных внутри <code>/config</code>, внутренний API — на порту 5000. В этой версии используется CPU-профиль без проброса GPU и USB-устройств.</p>
      <label htmlFor="frigate-config">Конфигурация</label>
      <textarea id="frigate-config" className="text-input mono" rows={22} spellCheck={false} disabled={busy} value={configText} onChange={event => setConfigText(event.target.value)} />
      <div className="button-row">
        <button type="button" className="button primary" disabled={busy || !configText.trim()} onClick={() => setConfirmSave(true)}>Проверить и сохранить</button>
        <button type="button" className="button secondary" disabled={busy} onClick={() => {setEditing(false); setConfigText("");}}>Отмена</button>
      </div>
    </Panel>}
    <p className="muted small">Кадры камер обновляются каждые 5 секунд. Записи хранятся отдельно от базы и конфигурации. Удаление модуля не удаляет его данные.</p>
    {confirmSave && <ConfirmDialog title="Сохранить конфигурацию Frigate?" confirmLabel="Проверить и сохранить" onCancel={() => setConfirmSave(false)} onConfirm={() => void saveConfig()}>
      <p>Frigate проверит YAML до сохранения. Изменения не применяются автоматически: перезапуск выполняется отдельно через очередь задач HomeAiCore.</p>
    </ConfirmDialog>}
    {confirmRestart && <ConfirmDialog title="Перезапустить Frigate?" confirmLabel="Перезапустить" onCancel={() => setConfirmRestart(false)} onConfirm={() => void restart()}>
      <p>Запись камер Frigate будет временно приостановлена. Существующий модуль NVR не затрагивается.</p>
    </ConfirmDialog>}
  </div>;
}
function eventURL(id: string, file: string) {return `/api/v1/frigate/api/events/${encodeURIComponent(id)}/${file}`;}
function Camera({name, config, stats, frame}: {name: string; config: FrigateConfig; stats: FrigateStats; frame: number}) {
  const state = stats.cameras?.[name];
  return <article className="module-card"><h3>{name}</h3>
    <p>{config.cameras[name]?.enabled === false ? "Камера отключена" : `Поток: ${state?.camera_fps ?? "—"} кадров/с · детекция: ${state?.detection_fps ?? "—"} кадров/с`}</p>
    <img alt={`Камера ${name}`} src={`/api/v1/frigate/api/snapshot/${encodeURIComponent(name)}?frame=${frame}`} style={{width: "100%", objectFit: "contain"}} />
  </article>;
}
