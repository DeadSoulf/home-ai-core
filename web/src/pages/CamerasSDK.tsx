import {FormEvent, useCallback, useState} from "react";
import {api} from "../api/client";
import type {Actor} from "../api/types";
import {ErrorState, LoadingState, Panel} from "../components/Panel";
import {useResource} from "../hooks/useResource";
import {useI18n} from "../i18n";
import {PageHeading} from "./Dashboard";

export function CamerasSDKPage({revision, actor}: {revision: number; actor: Actor}) {
  const {t} = useI18n();
  const load = useCallback(() => api.camerasStatus(), []);
  const resource = useResource(load, revision);
  const canManage = actor.permissions.includes("camera.manage");

  const [devices, setDevices] = useState<Array<{address: string; port: number; name?: string; scopes?: string; xaddr?: string}>>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState("");
  const [sdkFile, setSDKFile] = useState<File>();
  const [installing, setInstalling] = useState(false);
  const [installError, setInstallError] = useState("");
  const [uploadProgress, setUploadProgress] = useState(0);
  const [adding, setAdding] = useState<string>("");
  const [addError, setAddError] = useState("");
  const [credentials, setCredentials] = useState<{address: string; port: number; name: string; username: string; password: string} | null>(null);
  const [savedCameras, setSavedCameras] = useState<Array<{id: string; name: string; address: string; port: number; username: string; backend: string}>>([]);

  const installSDK = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage || !sdkFile || installing) return;
    setInstalling(true);
    setInstallError("");
    setUploadProgress(0);
    try {
      await api.installCameraSDK(sdkFile, setUploadProgress);
      await resource.reload();
    } catch (reason) {
      setInstallError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setInstalling(false);
    }
  };

  const addCamera = async (event: FormEvent) => {
    event.preventDefault();
    if (!credentials || adding) return;
    setAdding(credentials.address);
    setAddError("");
    try {
      await api.addSDKCamera({
        name: credentials.name,
        address: credentials.address,
        port: credentials.port,
        username: credentials.username,
        password: credentials.password,
      });
      setSavedCameras(await api.camerasList());
      setCredentials(null);
    } catch (reason) {
      setAddError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setAdding("");
    }
  };

  const discover = async () => {
    if (searching) return;
    setSearching(true);
    setSearchError("");
    try {
      setDevices(await api.discoverSDKCameras());
    } catch (reason) {
      setSearchError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setSearching(false);
    }
  };

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  const status = resource.data!;
  const sdkReady = status.sdk.supported && status.sdk.available && status.sdk.initialized;

  return (
    <div className="page">
      <PageHeading title={t("cameras")} subtitle={t("camerasSDKSubtitle")} />
      {resource.error && <ErrorState message={resource.error} />}

      <Panel title={t("camerasSDKRuntime")}>
        <div className={sdkReady ? "notice success" : "notice warning"}>
          {sdkReady ? t("camerasSDKReady") : t("camerasSDKNotReady")}
        </div>
        <dl className="details">
          <dt>{t("camerasSDKBackend")}</dt><dd>{status.backend}</dd>
          <dt>{t("architecture")}</dt><dd className="mono">{status.sdk.architecture}</dd>
          <dt>{t("camerasSDKSupported")}</dt><dd>{status.sdk.supported ? t("yes") : t("no")}</dd>
          <dt>{t("camerasSDKAvailable")}</dt><dd>{status.sdk.available ? t("yes") : t("no")}</dd>
          <dt>{t("camerasSDKInitialized")}</dt><dd>{status.sdk.initialized ? t("yes") : t("no")}</dd>
          <dt>{t("version")}</dt><dd>{status.version}</dd>
          {status.sdk.library_path && (
            <><dt>{t("camerasSDKLibrary")}</dt><dd className="mono">{status.sdk.library_path}</dd></>
          )}
          {status.sdk.error && (
            <><dt>{t("camerasSDKError")}</dt><dd>{status.sdk.error}</dd></>
          )}
        </dl>
        {!sdkReady && status.sdk.supported && (
          <>
            <div className="notice">
              Загрузите оригинальный Linux64 ZIP-пакет HCNetSDK от Hikvision. Home AI Core установит runtime в своё защищённое хранилище автоматически.
            </div>
            {installError && <ErrorState message={installError} />}
            <form className="network-profile-form" onSubmit={installSDK}>
              <label>
                HCNetSDK Linux64 ZIP
                <input
                  type="file"
                  accept=".zip,application/zip"
                  onChange={(event) => setSDKFile(event.target.files?.[0])}
                  disabled={!canManage || installing}
                  required
                />
              </label>
              <button type="submit" className="button primary" disabled={!canManage || !sdkFile || installing}>
                {installing ? `Загрузка ${uploadProgress}%` : "Установить HCNetSDK"}
              </button>
              {installing && (
                <div>
                  <progress value={uploadProgress} max={100} style={{width: "100%"}} />
                  <div className="muted">{uploadProgress < 100 ? `Загружено: ${uploadProgress}%` : "Загрузка завершена, устанавливаю SDK…"}</div>
                </div>
              )}
            </form>
          </>
        )}
        {credentials && (
          <form className="network-profile-form" onSubmit={addCamera}>
            <h3>Добавление камеры {credentials.address}</h3>
            {addError && <ErrorState message={addError} />}
            <label>Название<input value={credentials.name} onChange={(e) => setCredentials({...credentials, name: e.target.value})} /></label>
            <label>Логин<input value={credentials.username} onChange={(e) => setCredentials({...credentials, username: e.target.value})} required /></label>
            <label>Пароль<input type="password" value={credentials.password} onChange={(e) => setCredentials({...credentials, password: e.target.value})} /></label>
            <div className="actions">
              <button type="submit" className="button primary" disabled={!!adding}>{adding ? "Подключение…" : "Добавить через HCNetSDK"}</button>
              <button type="button" className="button" onClick={() => setCredentials(null)} disabled={!!adding}>Отмена</button>
            </div>
            <p className="muted">Hikvision/HiWatch, найденные через SADP, используют обнаруженный SDK-порт. Для камер, найденных только через ONVIF, по умолчанию используется HCNetSDK-порт 8000.</p>
          </form>
        )}
        {savedCameras.length > 0 && (
          <div className="notice success">Добавлено камер: {savedCameras.length}. Следующий этап — получение каналов и Live View.</div>
        )}
      </Panel>

      <Panel title="Камеры">
        <p className="muted">Поиск доступных ONVIF-камер в локальной сети. Hikvision/HiWatch после добавления будут работать через HCNetSDK.</p>
        {searchError && <ErrorState message={searchError} />}
        <button type="button" className="button primary" onClick={discover} disabled={searching || !sdkReady}>
          {searching ? "Поиск…" : "Поиск"}
        </button>
        {!searching && devices.length === 0 && <p className="muted">Нажмите «Поиск», чтобы найти камеры.</p>}
        {devices.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead><tr><th>Камера</th><th>IP</th><th>ONVIF</th><th></th></tr></thead>
              <tbody>
                {devices.map((device) => (
                  <tr key={device.address}>
                    <td>{device.name || "ONVIF камера"}</td>
                    <td className="mono">{device.address}</td>
                    <td>{device.port}</td>
                    <td><button type="button" className="button" disabled={!canManage} onClick={() => setCredentials({address: device.address, port: device.scopes?.startsWith("hikvision:sadp") ? device.port : 8000, name: device.name || "", username: "admin", password: ""})}>Добавить</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>
    </div>
  );
}
