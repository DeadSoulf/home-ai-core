import {useEffect, useRef, useState} from "react";
import Hls from "hls.js";
import {api} from "../api/client";
import type {NVRCamera} from "../api/types";

export function NVRLivePlayer({camera}: {camera: NVRCamera}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [phase, setPhase] = useState<"starting" | "playing" | "error">("starting");
  const [error, setError] = useState("");

  useEffect(() => {
    let disposed = false;
    let hls: Hls | undefined;
    const cameraID = camera.id;
    const attach = async () => {
      setPhase("starting");
      setError("");
      try {
        const stream = await api.nvrLiveCamera(cameraID, "start");
        if (disposed) {
          void api.nvrLiveCamera(cameraID, "stop").catch(() => undefined);
          return;
        }
        const video = videoRef.current;
        if (!video || !stream.playlist_url) throw new Error("NVR не вернул ссылку на видеопоток");
        if (Hls.isSupported()) {
          hls = new Hls({
            enableWorker: true, lowLatencyMode: true,
            liveSyncDurationCount: 3, maxBufferLength: 6,
            xhrSetup: (xhr) => { xhr.withCredentials = true; },
          });
          hls.on(Hls.Events.ERROR, (_, data) => {
            if (!disposed && data.fatal) {
              setError("Видеопоток прервался: " + data.details);
              setPhase("error");
            }
          });
          hls.attachMedia(video);
          hls.loadSource(stream.playlist_url);
        } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
          video.src = stream.playlist_url;
        } else {
          throw new Error("Браузер не поддерживает воспроизведение HLS");
        }
        setPhase("playing");
        void video.play().catch(() => undefined);
      } catch (reason) {
        if (!disposed) {
          setPhase("error");
          setError(reason instanceof Error ? reason.message : "Не удалось открыть видеопоток");
        }
      }
    };
    void attach();
    return () => {
      disposed = true;
      hls?.destroy();
      const video = videoRef.current;
      if (video) {
        video.pause();
        video.removeAttribute("src");
        video.load();
      }
      void api.nvrLiveCamera(cameraID, "stop").catch(() => undefined);
    };
  }, [camera.id]);

  return <div className="nvr-live-tile">
    <div className="nvr-live-heading">
      <strong>{camera.name}</strong><span className="muted small">{camera.ip || camera.source_type}</span>
    </div>
    <video ref={videoRef} controls muted autoPlay playsInline preload="none" aria-label={"Прямой эфир: " + camera.name} />
    {phase === "starting" && <p role="status" className="muted small">Подключаемся к камере…</p>}
    {phase === "error" && <p role="alert" className="nvr-notice-error small">{error}</p>}
  </div>;
}
