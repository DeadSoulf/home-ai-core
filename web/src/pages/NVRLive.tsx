import {useEffect, useRef, useState} from "react";
import Hls from "hls.js";
import {api} from "../api/client";
import type {NVRCamera} from "../api/types";
import {Panel} from "../components/Panel";

export function NVRLive({cameras, canManage}: {cameras: NVRCamera[]; canManage: boolean}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const activeRef = useRef<string | undefined>(undefined);
  const [cameraID, setCameraID] = useState("");
  const [playlist, setPlaylist] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    return () => {
      const active = activeRef.current;
      if (active) void api.nvrLiveStop(active).catch(() => {});
    };
  }, []);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !playlist) return;
    let hls: Hls | undefined;
    const onError = (_event: unknown, data: {fatal: boolean}) => {
      if (data.fatal) setError("Ошибка воспроизведения видеопотока. Проверьте камеру и сеть.");
    };
    if (Hls.isSupported()) {
      hls = new Hls({
        enableWorker: true,
        lowLatencyMode: true,
        liveSyncDurationCount: 3,
        maxBufferLength: 8,
      });
      hls.on(Hls.Events.ERROR, onError);
      hls.loadSource(playlist);
      hls.attachMedia(video);
    } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = playlist;
    } else {
      setError("Браузер не поддерживает HLS-видеопоток.");
    }
    return () => {
      hls?.destroy();
      video.pause();
      video.removeAttribute("src");
      video.load();
    };
  }, [playlist]);

  async function stop() {
    const current = activeRef.current;
    setPlaylist("");
    setBusy(Boolean(current));
    activeRef.current = undefined;
    setCameraID("");
    try {
      if (current) await api.nvrLiveStop(current);
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось остановить видеопоток");
    } finally {
      setBusy(false);
    }
  }

  async function start(camera: NVRCamera) {
    if (!canManage || busy || !camera.enabled) return;
    setBusy(true);
    setError("");
    setPlaylist("");
    try {
      if (activeRef.current) {
        await api.nvrLiveStop(activeRef.current);
        activeRef.current = undefined;
      }
      const response = await api.nvrLiveStart(camera.id);
      if (!response.ok || !response.playlist_url.startsWith("/api/v1/nvr/output/hls/")) {
        throw new Error("NVR вернул некорректный адрес видеопотока");
      }
      activeRef.current = camera.id;
      setCameraID(camera.id);
      setPlaylist(response.playlist_url);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Невозможно получить видео с камеры");
    } finally { setBusy(false); }
  }

  const currentCamera = cameras.find((camera) => camera.id === cameraID);

  return <Panel title="Просмотр в реальном времени">
    <p className="muted">
      Выберите камеру. NVR создаст HLS-поток на сервере, используя сохранённые настройки
      Hikvision или RTSP. Пароль камеры в браузер не передаётся.
    </p>
    {!canManage && <p className="muted">Для запуска видеотрансляции требуются права управления NVR.</p>}
    <div className="nvr-live-layout">
      <div className="nvr-live-camera-list" aria-label="Выбор камеры для просмотра">
        {cameras.map((camera) => <button type="button" key={camera.id}
          className={cameraID === camera.id ? "nvr-live-camera selected" : "nvr-live-camera"}
          disabled={!canManage || busy || !camera.enabled}
          onClick={() => void start(camera)}>
          <strong>{camera.name}</strong>
          <small>{camera.source_type === "hikvision" ? "Hikvision" : "RTSP"} ·
            {camera.enabled ? " Доступна" : " Отключена"}</small>
        </button>)}
        {cameras.length === 0 && <p className="muted">Добавьте камеру на вкладке «Камеры».</p>}
      </div>
      <div className="nvr-live-workspace">
        <div className="nvr-live-video">
          {playlist && <video ref={videoRef} autoPlay muted controls playsInline
            aria-label={"Прямой эфир: " + (currentCamera?.name || "камера")} />}
          {!playlist && <div className="nvr-live-placeholder">
            {busy ? "Подключение к камере…" : "Выберите камеру, чтобы начать просмотр"}
          </div>}
        </div>
        <div className="nvr-live-toolbar">
          <span>{playlist ? "● LIVE · " + (currentCamera?.name || cameraID) : "Нет активного потока"}</span>
          <button type="button" className="button secondary" disabled={!cameraID && !busy}
            onClick={() => void stop()}>Остановить просмотр</button>
        </div>
      </div>
    </div>
    {error && <p role="alert" className="nvr-notice-error">{error}</p>}
    <p className="muted small">
      Если видео не запускается, проверьте RTSP-путь, логин камеры и поддержку кодека.
      Для запуска по сохранённому ID необходим NVR версии 1.1.3 или новее.
    </p>
  </Panel>;
}
