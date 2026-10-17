import {useCallback, useEffect, useState, type FormEvent} from "react";
import {api} from "../api/client";
import type {NVRCamera, NVRRecording, NVRStatus, NVRDiscoveryNetwork, NVRDiscoveredCamera} from "../api/types";
import {Panel} from "../components/Panel";
import {PageHeading} from "./Dashboard";
import {ConfirmDialog} from "../components/ConfirmDialog";
import {NVRLive} from "./NVRLive";
import {NVRStorage} from "./NVRStorage";
import "./NVR.css";

type NVRTab = "cameras" | "live" | "discovery" | "archive" | "storage";

const localDate = (seconds: number) => new Date(seconds * 1000).toLocaleString();
const bytes = (value: number) => value >= 1024 ** 3
  ? (value / (1024 ** 3)).toFixed(1) + " ГБ"
  : (value / (1024 ** 2)).toFixed(1) + " МБ";

export function NVRPage({canManage, canManageStorage = false, onNavigate = () => {}}: {
  canManage: boolean;
  canManageStorage?: boolean;
  onNavigate?: (path: string) => void;
}) {
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
  const [deleteTarget, setDeleteTarget] = useState<NVRCamera>();
  const [deleteArchive, setDeleteArchive] = useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [networks, setNetworks] = useState<NVRDiscoveryNetwork[]>([]);
  const [selectedNetwork, setSelectedNetwork] = useState("");
  const [discovered, setDiscovered] = useState<NVRDiscoveredCamera[]>([]);
  const [discoveryError, setDiscoveryError] = useState("");
  const [discoveryLoading, setDiscoveryLoading] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [scanSummary, setScanSummary] = useState("");
  const [hasScanned, setHasScanned] = useState(false);

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
    if (busy) return false;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await task();
      await reload();
      setMessage(success);
      return true;
    } catch (err) {
      setError(err instanceof Error ? err.message : "Операция NVR завершилась ошибкой");
      return false;
    } finally { setBusy(false); }
  };

  async function createCamera(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const saved = await act(() => api.nvrAddCamera({
      name: name.trim(), source_type: sourceType,
      ...(sourceType === "rtsp"
        ? {rtsp_url: address.trim()}
        : {ip: address.trim(), port, username: username.trim(), password, channel}),
    }), "Камера добавлена");
    // Clear credentials even if the server rejected the camera.
    setPassword("");
    if (saved) {
      setAdding(false);
      setName("");
      setAddress("");
    }
  }

  useEffect(() => {
    if (tab !== "discovery" || !canManage) return;
    const controller = new AbortController();
    setDiscoveryLoading(true);
    void api.nvrDiscoveryNetworks(controller.signal).then((available) => {
      if (controller.signal.aborted) return;
      setNetworks(available);
      setSelectedNetwork((old) => available.some((net) => net.id === old) ? old : available[0]?.id || "");
      setDiscoveryError("");
    }).catch((err: unknown) => {
      if (!controller.signal.aborted) {
        setDiscoveryError(err instanceof Error ? err.message : "Не удалось получить список локальных сетей");
      }
    }).finally(() => {
      if (!controller.signal.aborted) setDiscoveryLoading(false);
    });
    return () => controller.abort();
  }, [tab, canManage]);

  async function searchForCameras() {
    if (!selectedNetwork || scanning || !canManage) return;
    setScanning(true);
    setDiscoveryError("");
    setScanSummary("");
    setHasScanned(false);
    setDiscovered([]);
    try {
      const result = await api.nvrDiscover(selectedNetwork);
      setDiscovered(result.devices);
      setHasScanned(true);
      setScanSummary(`Проверено до ${result.scanned_hosts} адресов · ${(result.duration_ms / 1000).toFixed(1)} с`);
    } catch (err) {
      setDiscoveryError(err instanceof Error ? err.message : "Ошибка поиска камер");
    } finally {
      setScanning(false);
    }
  }

  function configureFoundCamera(device: NVRDiscoveredCamera) {
    const viaHikvision = device.vendor === "Hikvision" && device.sdk_port === 8000;
    setSourceType(viaHikvision ? "hikvision" : "rtsp");
    setName((device.name || (device.vendor === "Не определён" ? "Камера" : device.vendor)) + " " + device.address);
    setAddress(viaHikvision ? device.address : `rtsp://${device.address}:${device.rtsp_port || 554}/`);
    setPort(8000);
    setChannel(1);
    setUsername("");
    setPassword("");
    setMessage(viaHikvision
      ? "Найдена камера Hikvision. Укажите логин и пароль для подключения через HCNetSDK."
      : "Проверьте RTSP-путь для этой модели и укажите учётные данные, если они требуются.");
    setDiscoveryError("");
    setAdding(true);
    setTab("cameras");
  }

  async function confirmDeleteCamera() {
    if (!deleteTarget || busy) return;
    if (deleteArchive && deleteConfirmation !== deleteTarget.name) {
      setError("Введите название камеры точно, чтобы подтвердить удаление видеоархива.");
      return;
    }
    const deleted = await act(() => api.nvrDeleteCamera(deleteTarget.id, deleteArchive),
      deleteArchive ? "Камера и её записи удалены." : "Камера удалена. Видеоархив сохранён.");
    if (deleted) {
      setDeleteTarget(undefined);
      setDeleteArchive(false);
      setDeleteConfirmation("");
    }
  }

  const archiveSelected = selected && recordings.some((item) => item.id === selected.id) ? selected : undefined;

  return <div className="page nvr-page">
    <PageHeading title="NVR · Видеонаблюдение" subtitle="Камеры, запись и архив — внутри HOME AI Core" />
    <div className="button-row">
      <button type="button" className={"button " + (tab === "cameras" ? "primary" : "secondary")}
        onClick={() => setTab("cameras")}>Камеры</button>
      <button type="button" className={"button " + (tab === "live" ? "primary" : "secondary")}
        onClick={() => setTab("live")}>В реальном времени</button>
      {canManage && <button type="button" className={"button " + (tab === "discovery" ? "primary" : "secondary")}
        onClick={() => setTab("discovery")}>Поиск камер</button>}
      <button type="button" className={"button " + (tab === "archive" ? "primary" : "secondary")}
        onClick={() => setTab("archive")}>Архив записей</button>
      <button type="button" className={"button " + (tab === "storage" ? "primary" : "secondary")}
        onClick={() => setTab("storage")}>Хранилище</button>
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
            <button type="button" className="button danger" disabled={busy} onClick={() => {
              setDeleteTarget(camera);
              setDeleteArchive(false);
              setDeleteConfirmation("");
            }}>Удалить камеру</button>
          </div>}
        </article>)}
      </div>
    </Panel>}

    {tab === "live" && <NVRLive cameras={cameras} canManage={canManage} />}
    {tab === "storage" && <NVRStorage canManage={canManage}
      canManageStorage={canManageStorage} onNavigate={onNavigate} />}

    {tab === "discovery" && canManage && <Panel title="Поиск сетевых камер">
      <p className="muted">
        Найдите камеры разных производителей без обязательного ONVIF: RTSP, WS-Discovery, SSDP,
        mDNS и сервисные порты. Обнаружение работает на сервере HOME AI Core, а не в браузере.
      </p>
      <div className="nvr-discovery-toolbar">
        <label>Локальная сеть
          <select value={selectedNetwork} disabled={scanning || discoveryLoading}
            onChange={(event) => {
              setSelectedNetwork(event.target.value);
              setDiscovered([]);
              setHasScanned(false);
              setScanSummary("");
            }}>
            {networks.length === 0 && <option value="">Не найдены доступные сети</option>}
            {networks.map((network) => <option key={network.id} value={network.id}>
              {network.interface} — {network.cidr} ({network.address})
            </option>)}
          </select>
        </label>
        <button className="button primary" type="button"
          disabled={!selectedNetwork || scanning || discoveryLoading}
          onClick={() => void searchForCameras()}>
          {scanning ? "Поиск камер…" : "Найти камеры"}
        </button>
      </div>
      {discoveryLoading && <p role="status">Определяем доступные сетевые интерфейсы…</p>}
      {scanning && <p role="status">Сканирование локальной сети. Поиск может занять до 15 секунд.</p>}
      {discoveryError && <p role="alert" className="nvr-notice-error">{discoveryError}</p>}
      {hasScanned && <p role="status" className="muted">{scanSummary} · Найдено устройств: {discovered.length}</p>}
      {hasScanned && discovered.length === 0 && <p className="muted">
        Камеры не обнаружены автоматически. Проверьте выбранную подсеть, VLAN и настройки сети
        камеры либо добавьте источник вручную на вкладке «Камеры».
      </p>}
      <div className="nvr-discovery-list">
        {discovered.map((camera) => <article className="nvr-discovery-device" key={camera.address}>
          <div className="nvr-discovery-main">
            <div className="nvr-discovery-title">
              <strong>{camera.name || camera.vendor}</strong>
              <span className="muted small">{camera.confidence === "detected" ? "Сервис обнаружен" : "Возможное устройство"}</span>
            </div>
            <p className="mono small">{camera.address} · {camera.vendor}</p>
            <div className="nvr-discovery-services">
              {camera.services.map((service) => <span className="tag" key={service}>{service}</span>)}
            </div>
          </div>
          <button type="button" className="button secondary" onClick={() => configureFoundCamera(camera)}>
            Настроить подключение
          </button>
        </article>)}
      </div>
      <p className="muted small">
        Важное ограничение: некоторые камеры используют закрытые протоколы, не отвечают на
        обнаружение или находятся в другой VLAN. Найденное устройство не означает, что
        видеопоток будет доступен без адреса RTSP-потока и учётных данных.
        Поиск ограничен напрямую подключённой локальной подсетью, не более /24,
        и никогда не перебирает пароли.
      </p>
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
    {deleteTarget && <ConfirmDialog title={`Удалить камеру «${deleteTarget.name}»?`}
      confirmLabel={deleteArchive ? "Удалить камеру и все записи" : "Удалить камеру"}
      onCancel={() => { setDeleteTarget(undefined); setDeleteArchive(false); setDeleteConfirmation(""); }}
      onConfirm={() => void confirmDeleteCamera()}>
      <p>По умолчанию камера удаляется из списка, но её архив сохраняется.</p>
      <label className="nvr-delete-archive">
        <input type="checkbox" checked={deleteArchive} onChange={(event) => {
          setDeleteArchive(event.target.checked);
          setDeleteConfirmation("");
        }} />
        Также безвозвратно удалить видеоархив этой камеры
      </label>
      {deleteArchive && <label className="nvr-delete-confirmation">
        Для удаления файлов введите название камеры: <strong>{deleteTarget.name}</strong>
        <input value={deleteConfirmation} onChange={(event) => setDeleteConfirmation(event.target.value)} />
      </label>}
      {error && <p role="alert" className="nvr-notice-error">{error}</p>}
    </ConfirmDialog>}
    <p className="muted small">Доступ к NVR защищён сессией HOME AI Core. Порт контейнера не опубликован наружу.</p>
  </div>;
}
