import {useCallback, useEffect, useState, type FormEvent} from "react";
import {api} from "../api/client";
import type {NVRCamera, NVRRecording, NVRStatus} from "../api/types";
import {Panel} from "../components/Panel";
import {PageHeading} from "./Dashboard";
import "./NVR.css";

type NVRTab = "cameras" | "archive";

const localDate = (seconds: number) => new Date(seconds * 1000).toLocaleString();
const bytes = (value: number) => value >= 1024 ** 3
  ? (value / (1024 ** 3)).toFixed(1) + " ГБ"
  : (value / (1024 ** 2)).toFixed(1) + " МБ";

export function NVRPage({canManage}: {canManage: boolean}) {
  const [tab, setTab] = useState<NVRTab>("cameras");
  const [status, setStatus] = useState<NVRStatus>();
  const [cameras, setCameras] = useState<NVRCamera[]>([]);
  const [recordings, setRecordings] = useState<NVRRecording[]>([]);
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<NVRRecording>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [sourceType, setSourceType] = useState<"hikvision" | "rtsp">("rtsp");
  const [address, setAddress] = useState("");
  const [port, setPort] = useState(8000);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [channel, setChannel] = useState(1);

  const reload = useCallback(async (signal?: AbortSignal) => {
    const [nextStatus, nextCameras, nextRecordings] = await Promise.all([
      api.nvrStatus(signal), api.nvrCameras(signal), api.nvrRecordings(filter || undefined, signal),
    ]);
    setStatus(nextStatus);
    setCameras(nextCameras);
    setRecordings(nextRecordings);
  }, [filter]);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    const run = async () => {
      try {
        await reload(controller.signal);
        if (active) setError("");
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : "Не удалось подключиться к NVR");
      } finally { if (active) setLoading(false); }
    };
    void run();
    const timer = window.setInterval(() => { void run(); }, 15000);
    return () => { active = false; controller.abort(); clearInterval(timer); };
  }, [reload]);

  const act = async (task: () => Promise<unknown>, success: string) => {
    if (busy) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await task();
      await reload();
      setMessage(success);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Операция NVR завершилась ошибкой");
    } finally { setBusy(false); }
  };

  async function createCamera(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await act(() => api.nvrAddCamera({
      name: name.trim(), source_type: sourceType,
      ...(sourceType === "rtsp"
        ? {rtsp_url: address.trim()}
        : {ip: address.trim(), port, username: username.trim(), password, channel}),
    }), "Камера добавлена");
    // Clear credentials even if the server rejected the camera.
    setPassword("");
    setAdding(false);
    setName("");
    setAddress("");
  }

  const archiveSelected = selected && recordings.some((item) => item.id === selected.id) ? selected : undefined;

  return <div className="page nvr-page">
    <PageHeading title="NVR · Видеонаблюдение" subtitle="Камеры, запись и архив — внутри HOME AI Core" />
    <div className="button-row">
      <button type="button" className={"button " + (tab === "cameras" ? "primary" : "secondary")}
        onClick={() => setTab("cameras")}>Камеры</button>
      <button type="button" className={"button " + (tab === "archive" ? "primary" : "secondary")}
        onClick={() => setTab("archive")}>Архив записей</button>
      <button type="button" className="button secondary" disabled={busy}
        onClick={() => { setLoading(true); void reload().then(() => setError("")).catch((e: unknown) => setError(String(e))).finally(() => setLoading(false)); }}>
        Обновить
      </button>
    </div>
    {error && <p role="alert" className="nvr-notice-error">{error}</p>}
    {message && <p role="status" className="nvr-notice-success">{message}</p>}
    {loading && !status && <p role="status">Подключение к NVR…</p>}
    {status && <div className="nvr-stat-grid">
      <div className="nvr-stat"><small>Камеры</small><strong>{status.cameras} / {status.max_cameras}</strong></div>
      <div className="nvr-stat"><small>Ведётся запись</small><strong>{status.active_recorders}</strong></div>
      <div className="nvr-stat"><small>Файлы архива</small><strong>{status.recordings}</strong></div>
      <div className="nvr-stat"><small>Занято на диске</small><strong>{bytes(status.storage_bytes)}</strong></div>
    </div>}

    {tab === "cameras" && <Panel title="Подключённые камеры">
      <div className="button-row">
        {canManage && <button type="button" className="button primary" onClick={() => setAdding((value) => !value)}>
          {adding ? "Закрыть форму" : "Добавить камеру"}
        </button>}
      </div>
      {canManage && adding && <form className="nvr-camera-form" onSubmit={(event) => { void createCamera(event); }}>
        <label>Название камеры<input required maxLength={128} value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label>Тип источника<select value={sourceType} onChange={(e) => setSourceType(e.target.value as "hikvision" | "rtsp")}>
          <option value="rtsp">RTSP</option><option value="hikvision">Hikvision HCNetSDK</option>
        </select></label>
        <label>{sourceType === "rtsp" ? "RTSP URL" : "IP-адрес камеры"}
          <input required value={address} onChange={(e) => setAddress(e.target.value)}
            placeholder={sourceType === "rtsp" ? "rtsp://192.168.1.10/stream" : "192.168.1.10"} />
        </label>
        {sourceType === "hikvision" && <>
          <label>Порт<input type="number" min={1} max={65535} value={port} onChange={(e) => setPort(Number(e.target.value))} /></label>
          <label>Пользователь<input required value={username} onChange={(e) => setUsername(e.target.value)} /></label>
          <label>Пароль<input type="password" required autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} /></label>
          <label>Канал<input type="number" min={1} value={channel} onChange={(e) => setChannel(Number(e.target.value))} /></label>
        </>}
        <div className="button-row"><button type="submit" className="button primary" disabled={busy || !name.trim() || !address.trim()}>
          {busy ? "Сохранение…" : "Подключить камеру"}
        </button></div>
      </form>}
      {cameras.length === 0 && <p className="muted">Камер пока нет. Добавьте RTSP-источник или устройство Hikvision.</p>}
      <div className="nvr-camera-list">
        {cameras.map((camera) => <article key={camera.id} className="nvr-camera">
          <div>
            <strong>{camera.name}</strong>
            <p className="muted small">{camera.source_type === "rtsp" ? "RTSP" : "Hikvision"} · {camera.ip || "Сетевой поток"}
              {camera.channel ? " · канал " + camera.channel : ""}</p>
            <p className="small"><span className={camera.recording ? "nvr-active" : "muted"}>
              {camera.recording ? "● Идёт запись" : camera.enabled ? "○ Запись остановлена" : "○ Выключена"}
            </span></p>
          </div>
          {canManage && <div className="button-row">
            {camera.enabled && <button type="button" className="button secondary" disabled={busy} onClick={() => {
              void act(() => api.nvrControlCamera(camera.id, camera.recording ? "stop" : "start"), "Команда выполнена");
            }}>{camera.recording ? "Остановить" : "Запустить"}</button>}
            {camera.enabled && <button type="button" className="button secondary" disabled={busy} onClick={() => {
              void act(() => api.nvrControlCamera(camera.id, "restart"), "Камера перезапущена");
            }}>Перезапустить</button>}
          </div>}
        </article>)}
      </div>
    </Panel>}

    {tab === "archive" && <Panel title="Архив видеозаписей">
      <div className="nvr-archive-filter">
        <label>Камера
          <select value={filter} onChange={(e) => { setFilter(e.target.value); setSelected(undefined); }}>
            <option value="">Все камеры</option>
            {cameras.map((camera) => <option key={camera.id} value={camera.id}>{camera.name}</option>)}
          </select>
        </label>
      </div>
      {archiveSelected?.play_url && <div className="nvr-player">
        <video key={archiveSelected.play_url} controls playsInline preload="metadata"
          src={archiveSelected.play_url} aria-label="Видеозапись из архива" />
        <p className="muted small">Запись от {localDate(archiveSelected.started_at)}</p>
      </div>}
      {recordings.length === 0 && <p className="muted">Записей пока нет.</p>}
      <div className="nvr-camera-list">
        {recordings.map((recording) => <article key={recording.id} className="nvr-camera">
          <div><strong>{localDate(recording.started_at)}</strong>
            <p className="muted small">{cameras.find((camera) => camera.id === recording.camera_id)?.name || recording.camera_id}
              {" · " + bytes(recording.size_bytes)}
            </p>
          </div>
          {recording.play_url && <button type="button" className="button secondary"
            onClick={() => setSelected(recording)}>Просмотреть</button>}
        </article>)}
      </div>
    </Panel>}
    <p className="muted small">Доступ к NVR защищён сессией HOME AI Core. Порт контейнера не опубликован наружу.</p>
  </div>;
}
