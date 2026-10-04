import {useCallback} from "react";
import {api} from "../api/client";
import {ErrorState, LoadingState, Panel} from "../components/Panel";
import {useResource} from "../hooks/useResource";
import {PageHeading} from "./Dashboard";

export function CamerasViewerPage({revision, onNavigate}: {revision: number; onNavigate: (path: string) => void}) {
  const load = useCallback(() => api.camerasStatus(), []);
  const resource = useResource(load, revision);

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  const status = resource.data!;
  if (!status.websdk.available) {
    return (
      <div className="page">
        <PageHeading title="Просмотр камер" subtitle="Live и архив через оригинальный Hikvision WebSDK V3.3.1." />
        <Panel title="WebSDK не установлен">
          <div className="notice warning">
            Загрузите оригинальный WebSDK V3.3.1 ZIP в настройках Cameras. После установки эта страница использует vendor webVideoCtrl.js и HCWebSDKPlugin без переписывания player logic.
          </div>
          <button type="button" className="button primary" onClick={() => onNavigate("/modules/cameras")}>
            Открыть настройки Cameras
          </button>
        </Panel>
      </div>
    );
  }

  return (
    <div className="page camera-viewer-page">
      <PageHeading title="Просмотр камер" subtitle="Оригинальный Hikvision WebSDK V3.3.1: Live и архив." />
      {resource.error && <ErrorState message={resource.error} />}
      <Panel title="WebSDK runtime">
        <div className="camera-viewer-status">
          <span>WebSDK: <strong>V{status.websdk.version || "3.3.1"}</strong></span>
          <span>Media: <strong>HCWebSDKPlugin</strong></span>
          <span>Режим: <strong>Live + Archive</strong></span>
        </div>
        <p className="muted">
          Viewer изолирован от основного UI. Камерные логины и пароли вводятся внутри Hikvision viewer и не отправляются в HOME AI Core.
        </p>
        <div className="button-row">
          <a className="button secondary" href={api.cameraWebSDKAssetURL("HCWebSDKPlugin.exe")}>
            Скачать HCWebSDKPlugin.exe
          </a>
          <button type="button" className="button secondary" onClick={() => onNavigate("/modules/cameras")}>
            Настройки Cameras
          </button>
        </div>
      </Panel>
      <iframe
        className="camera-websdk-frame"
        src="/api/v1/cameras/websdk/viewer"
        title="Hikvision WebSDK Live and Archive"
        allow="fullscreen"
      />
    </div>
  );
}
