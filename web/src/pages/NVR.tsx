import {useCallback, useEffect, useRef, useState, type FormEvent} from "react";
import {api, APIError} from "../api/client";
import type {NVRCamera, NVRRecording, NVRStatus, NVRDiscoveryNetwork, NVRDiscoveredCamera, NVRStorageStatus} from "../api/types";
import {Panel} from "../components/Panel";
import {NVRLivePlayer} from "../components/NVRLivePlayer";
import {NVRStoragePanel} from "../components/NVRStoragePanel";
import {PageHeading} from "./Dashboard";
import "./NVR.css";

type NVRTab = "cameras" | "live" | "discovery" | "archive";

const localDate = (seconds: number) => new Date(seconds * 1000).toLocaleString();
const bytes = (value: number) => value >= 1024 ** 3
  ? (value / (1024 ** 3)).toFixed(1) + " ГБ"
  : (value / (1024 ** 2)).toFixed(1) + " МБ";

export function NVRPage({canManage}: {canManage: boolean}) {
  const [tab, setTab] = useState<NVRTab>("cameras");
  const [status, setStatus] = useState<NVRStatus>();
  const [storage, setStorage] = useState<NVRStorageStatus>();
  const [storageLoading, setStorageLoading] = useState(true);
  const [storageError, setStorageError] = useState("");
  const [cameras, setCameras] = useState<NVRCamera[]>([]);
  const [recordings, setRecordings] = useState<NVRRecording[]>([]);
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<NVRRecording>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [adding, setAdding] = useState(false);
  const [liveCameraID, setLiveCameraID] = useState("");
  const [activeLiveID, setActiveLiveID] = useState("");
  const [livePlaylist, setLivePlaylist] = useState("");
  const [liveBusy, setLiveBusy] = useState(false);
  const [liveError, setLiveError] = useState("");
  const activeLiveRef = useRef("");
  const [name, setName] = useState("");
  const [sourceType, setSourceType] = useState<"hikvision" | "rtsp">("rtsp");
  const [address, setAddress] = useState("");
  const [port, setPort] = useState(8000);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [channel, setChannel] = useState(1);
  const [networks, setNetworks] = useState<NVRDiscoveryNetwork[]>([]);
  const [selectedNetwork, setSelectedNetwork] = useState("");
  const [discovered, setDiscovered] = useState<NVRDiscoveredCamera[]>([]);
  const [discoveryError, setDiscoveryError] = useState("");
  const [discoveryLoading, setDiscoveryLoading] = useState(false);
  const [discoveryRetry, setDiscoveryRetry] = useState(0);
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

  const refreshStorage = useCallback(async (signal?: AbortSignal) => {
    const next = await api.nvrStorage(signal);
    if (!signal?.aborted) {
      setStorage(next);
      setStorageError("");
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    let pending = false;
    const run = async () => {
      if (pending) return;
      pending = true;
      try {
        await refreshStorage(controller.signal);
      } catch (err) {
        if (active && !controller.signal.aborted) {
          setStorageError(err instanceof Error ? err.message : "Не удалось прочитать сведения о дисках NVR");
        }
      } finally {
        pending = false;
        if (active) setStorageLoading(false);
      }
    };
    void run();
    const timer = window.setInterval(() => { void run(); }, 30000);
    return () => { active = false; controller.abort(); clearInterval(timer); };
  }, [refreshStorage]);

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
  }, [tab, canManage, discoveryRetry]);

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

  useEffect(() => {
    if (!liveCameraID && cameras.length) setLiveCameraID(cameras[0].id);
  }, [cameras, liveCameraID]);

  useEffect(() => () => {
    const id = activeLiveRef.current;
    if (id) void api.nvrStopLive(id).catch(() => {});
  }, []);
  useEffect(() => {
    if (tab === "live" || !activeLiveRef.current) return;
    const id = activeLiveRef.current;
    activeLiveRef.current = "";
    setActiveLiveID("");
    setLivePlaylist("");
    void api.nvrStopLive(id).catch(() => {});
  }, [tab]);

  function formatNVRActionError(err: unknown): string {
    if (err instanceof APIError && err.code === "nvr_upgrade_required") {
      return "Для живого просмотра и безопасного удаления камеры необходим NVR 1.1.3 или новее. Обновите модуль, сохранив его данные.";
    }
    return err instanceof Error ? err.message : "Операция NVR завершилась ошибкой";
  }

  async function stopSavedLive() {
    const id = activeLiveRef.current;
    if (!id || liveBusy) return;
    setLiveBusy(true);
    setLiveError("");
    try {
      await api.nvrStopLive(id);
      activeLiveRef.current = "";
      setActiveLiveID("");
      setLivePlaylist("");
    } catch (err) {
      setLiveError(formatNVRActionError(err));
    } finally {
      setLiveBusy(false);
    }
  }

  async function startSavedLive() {
    if (!canManage || !liveCameraID || liveBusy) return;
    setLiveBusy(true);
    setLiveError("");
    try {
      if (activeLiveRef.current) {
        await api.nvrStopLive(activeLiveRef.current);
        activeLiveRef.current = "";
        setActiveLiveID("");
        setLivePlaylist("");
      }
      const result = await api.nvrStartLive(liveCameraID);
      if (!result.ok || !result.playlist_url.startsWith("/api/v1/nvr/output/hls/")) {
        throw new Error("NVR вернул некорректный адрес видеопотока");
      }
      activeLiveRef.current = liveCameraID;
      setActiveLiveID(liveCameraID);
      setLivePlaylist(result.playlist_url);
    } catch (err) {
      setLiveError(formatNVRActionError(err));
    } finally {
      setLiveBusy(false);
    }
  }

  async function removeCamera(camera: NVRCamera) {
    if (!canManage || busy) return;
    if (!window.confirm("Удалить камеру «" + camera.name + "» из списка? Сохранённые записи останутся в архиве.")) return;
    const success = await act(() => api.nvrDeleteCamera(camera.id), "Камера удалена. Архив сохранён.");
    if (success && activeLiveRef.current === camera.id) {
      activeLiveRef.current = "";
      setActiveLiveID("");
      setLivePlaylist("");
    }
    if (!success) setError("Не удалось удалить камеру. Проверьте доступ и версию модуля NVR (не ниже 1.1.3).");
  }

  const archiveSelected = selected && recordings.some((item) => item.id === selected.id) ? selected : undefined;

  return <div className="page nvr-page">
    <PageHeading title="NVR · Видеонаблюдение" subtitle="Камеры, запись и архив — внутри HOME AI Core" />
    <div className="button-row">
      <button type="button" className={"button " + (tab === "cameras" ? "primary" : "secondary")}
        onClick={() => setTab("cameras")}>Камеры</button>
      <button type="button" className={"button " + (tab === "live" ? "primary" : "secondary")}
        onClick={() => setTab("live")}>Просмотр в реальном времени</button>
      {canManage && <button type="button" className={"button " + (tab === "discovery" ? "primary" : "secondary")}
        onClick={() => setTab("discovery")}>Поиск камер</button>}
      <button type="button" className={"button " + (tab === "archive" ? "primary" : "secondary")}
        onClick={() => setTab("archive")}>Архив записей</button>
      <button type="button" className="button secondary" disabled={busy}
        onClick={() => {
          setLoading(true);
          setStorageLoading(true);
          void reload().then(() => setError("")).catch((e: unknown) => setError(String(e))).finally(() => setLoading(false));
          void refreshStorage().catch((e: unknown) => setStorageError(e instanceof Error ? e.message : String(e)))
            .finally(() => setStorageLoading(false));
        }}>
        Обновить
      </button>
    </div>
    {error && <p role="alert" className="nvr-notice-error">Состояние модуля NVR: {error}</p>
    {message && <p role="status" className="nvr-notice-success">{message}</p>}
    {loading && !status && <p role="status">Подключение к NVR…</p>}
    {status && <div className="nvr-stat-grid">
      <div className="nvr-stat"><small>Камеры</small><strong>{status.cameras} / {status.max_cameras}</strong></div>
      <div className="nvr-stat"><small>Ведётся запись</small><strong>{status.active_recorders}</strong></div>
      <div className="nvr-stat"><small>Файлы архива</small><strong>{status.recordings}</strong></div>
      <div className="nvr-stat"><small>Размер видеоархива</small><strong>{bytes(status.storage_bytes)}</strong></div>
    </div>}

    <NVRStoragePanel storage={storage} loading={storageLoading} error={storageError} />

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
            <button type="button" className="button danger" disabled={busy}
              onClick={() => void removeCamera(camera)}>Удалить камеру</button>
          </div>}
        </article>)}
      </div>
    </Panel>}

    {tab === "live" && <Panel title="Просмотр камер в реальном времени">
      <div className="nvr-live-toolbar">
        <label>Камера
          <select value={liveCameraID} disabled={liveBusy} onChange={(event) => setLiveCameraID(event.target.value)}>
            <option value="">Выбрать камеру</option>
            {cameras.filter((camera) => camera.enabled).map((camera) =>
              <option key={camera.id} value={camera.id}>{camera.name}</option>)}
          </select>
        </label>
        {canManage && <button type="button" className="button primary"
          disabled={!liveCameraID || liveBusy} onClick={() => void startSavedLive()}>
          {liveBusy ? "Подключение…" : "Смотреть"}
        </button>}
        {canManage && activeLiveID && <button type="button" className="button secondary"
          disabled={liveBusy} onClick={() => void stopSavedLive()}>Остановить просмотр</button>}
      </div>
      {liveError && <p role="alert" className="nvr-notice-error">{liveError}</p>}
      {livePlaylist && <NVRLivePlayer playlistURL={livePlaylist} />}
      {!livePlaylist && <p className="muted">Выберите сохранённую камеру и нажмите «Смотреть».
        Просмотр использует HLS через защищённый API HOME AI Core.</p>}
      {!canManage && <p className="muted small">Для запуска прямого видеопотока необходимы права управления NVR.</p>}
    </Panel>}

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
      {discoveryError && <div role="alert" className="nvr-notice-error">
        <p>Поиск камер: {discoveryError}</p>
        {!scanning && <button type="button" className="button secondary"
          onClick={() => setDiscoveryRetry((value) => value + 1)}>
          Повторить получение списка сетей
        </button>}
      </div>
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
    <p className="muted small">Доступ к NVR защищён сессией HOME AI Core. Порт контейнера не опубликован наружу.</p>
  </div>;
}
