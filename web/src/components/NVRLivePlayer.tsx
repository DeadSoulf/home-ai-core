import {useEffect, useRef, useState} from "react";
import Hls from "hls.js";

/** Authenticated HLS player: playlists and segments stay on Core's origin. */
export function NVRLivePlayer({playlistURL}: {playlistURL: string}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    setError("");
    if (Hls.isSupported()) {
      const hls = new Hls({enableWorker: true, lowLatencyMode: true, liveSyncDurationCount: 2});
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (data.fatal) setError("Не удалось воспроизвести поток. Проверьте соединение с камерой.");
      });
      hls.loadSource(playlistURL);
      hls.attachMedia(video);
      return () => {
        hls.destroy();
        video.removeAttribute("src");
        video.load();
      };
    }
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = playlistURL;
      video.load();
      return () => {
        video.pause();
        video.removeAttribute("src");
        video.load();
      };
    }
    setError("Этот браузер не поддерживает воспроизведение HLS.");
    return;
  }, [playlistURL]);

  return <div className="nvr-live-player">
    <video ref={videoRef} autoPlay muted playsInline controls aria-label="Живой видеопоток NVR" />
    {error && <p role="alert" className="nvr-notice-error">{error}</p>}
  </div>;
}
