import {useEffect, useRef, useState} from "react";
import Hls from "hls.js";
import {api} from "../api/client";
import type {NVRCamera} from "../api/types";
import {Panel} from "../components/Panel";

type LiveSession = {id: string; name: string; streamType: 0 | 1 | 2; url: string};

export function NVRLive({cameras, canManage}: {cameras: NVRCamera[]; canManage: boolean}) {
  const [session, setSession] = useState<LiveSession>();
  const [cameraID, setCameraID] = useState("");
  const [streamType, setStreamType] = useState<0 | 1 | 2>(1);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const videoRef = useRef<HTMLVideoElement>(null);
  const sessionRef = useRef<LiveSession>();

  const useSession = (next: LiveSession | undefined) => {
    sessionRef.current = next;
    setSession(next);
  };

  useEffect(() => () => {
    const current = sessionRef.current;
    if (current) void api.nvrLiveStop(current.id, current.streamType).catch(() => {});
  }, []);

  useEffect(() => {
    if (!session) return;
    const video = videoRef.current;
    if (!video) return;
    let hls: Hls | undefined;
    let stopped = false;
    const url = session.url;
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = url;
      void video.play().catch(() => {});
    } else if (Hls.isSupported()) {
      hls = new Hls({
        enableWorker: true, lowLatencyMode: true,
        liveSyncDurationCount: 3, maxBufferLength: 20,
      });
      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (data.fatal && !stopped) setError("Ошибка воспроизведения видеопотока: " + data.details);
      });
      hls.attachMedia(video);
      hls.on(Hls.Events.MEDIA_ATTACHED, () => {
        if (stopped) return;
        hls?.loadSource(url);
        void video.play().catch(() => {});
      });
    } else {
      setError("Этот браузер не поддерживает HLS-видео");
    }
    return () => {
      stopped = true;
      hls?.destroy();
      video.pause();
      video.removeAttribute("src");
      video.load();
      // Only the player is destroyed when switching streams. The stop API
      // is invoked by explicit control or the page-unmount cleanup.
    };
  }, [session]);

  async function start() {
    if (!canManage || busy || !cameraID) return;
    const camera = cameras.find((item) => item.id === cameraID);
    if (!camera) return;
    setBusy(true);
    setError("");
    try {
      if (session) {
        await api.nvrLiveStop(session.id, session.streamType).catch(() => {});
        useSession(undefined);
      }
      const result = await api.nvrLiveStart(camera.id, streamType);
      useSession({id: camera.id, name: camera.name, streamType, url: result.playlist_url});
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось запустить видеопоток");
    } finally {
      setBusy(false);
    }
  }

  async function stop() {
    const current = sessionRef.current;
    useSession(undefined);
    if (current) await api.nvrLiveStop(current.id, current.streamType).catch(() => {});
  }

  return <Panel title="Просмотр в реальном времени">
    <p className="muted small">
      Видеопоток передаётся через авторизованный сервер HOME AI Core. Запись камер
      продолжается независимо от просмотра. Для воспроизведения требуется NVR 1.1.3 или новее.
    </p>
    {canManage ? <div className="nvr-live-toolbar">
      <label>Камера
        <select value={cameraID} onChange={(event) => setCameraID(event.target.value)}>
          <option value="">Выберите камеру</option>
          {cameras.filter((item) => item.enabled).map((camera) =>
            <option value={camera.id} key={camera.id}>{camera.name}</option>)}
        </select>
      </label>
      <label>Поток
        <select value={streamType} onChange={(event) => setStreamType(Number(event.target.value) as 0 | 1 | 2)}>
          <option value={1}>Дополнительный / экономичный</option>
          <option value={0}>Основной / высокое качество</option>
          <option value={2}>Третий поток</option>
        </select>
      </label>
      <button type="button" className="button primary" disabled={!cameraID || busy}
        onClick={() => void start()}>
        {busy ? "Подключаем…" : "Смотреть"}
      </button>
      {session && <button type="button" className="button secondary"
        onClick={() => void stop()}>Остановить просмотр</button>}
    </div> : <p className="muted">Для запуска нового видеопотока нужны права управления NVR.</p>}
    {error && <p role="alert" className="nvr-notice-error">{error}</p>}
    {session && <div className="nvr-player nvr-live-player">
      <video ref={videoRef} playsInline controls muted autoPlay aria-label={"Прямая трансляция: " + session.name} />
      <p className="muted small">● LIVE · {session.name}</p>
    </div>}
    {cameras.length === 0 && <p className="muted">Сначала добавьте камеру во вкладке «Камеры».</p>}
  </Panel>;
}
