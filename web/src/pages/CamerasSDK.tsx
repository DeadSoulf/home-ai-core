import {FormEvent, useCallback, useState} from "react";
import {api} from "../api/client";
import type {Actor, CameraSDKLoginResult} from "../api/types";
import {ErrorState, LoadingState, Panel} from "../components/Panel";
import {useResource} from "../hooks/useResource";
import {useI18n} from "../i18n";
import {PageHeading} from "./Dashboard";

export function CamerasSDKPage({revision, actor}: {revision: number; actor: Actor}) {
  const {t} = useI18n();
  const load = useCallback(() => api.camerasStatus(), []);
  const resource = useResource(load, revision);
  const canManage = actor.permissions.includes("camera.manage");

  const [devices, setDevices] = useState<Array<{address: string; port: number; name?: string; xaddr?: string}>>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState("");
  const [sdkFile, setSDKFile] = useState<File>();
  const [installing, setInstalling] = useState(false);
  const [installError, setInstallError] = useState("");
  const [uploadProgress, setUploadProgress] = useState(0);
  const [probeAddress, setProbeAddress] = useState("");
  const [probePort, setProbePort] = useState("8000");
  const [probeUsername, setProbeUsername] = useState("admin");
  const [probePassword, setProbePassword] = useState("");
  const [probing, setProbing] = useState(false);
  const [probeError, setProbeError] = useState("");
  const [probeResult, setProbeResult] = useState<CameraSDKLoginResult>();

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

  const probeDevice = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage || probing) return;
    setProbing(true);
    setProbeError("");
    setProbeResult(undefined);
    try {
      const port = Number(probePort || "8000");
      const result = await api.testCameraSDKLogin({
        address: probeAddress.trim(),
        port,
        username: probeUsername.trim(),
        password: probePassword,
      });
      setProbeResult(result);
    } catch (reason) {
      setProbeError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setProbing(false);
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
              <thead><tr><th>Камера</th><th>IP</th><th>ONVIF/Web порт</th><th></th></tr></thead>
              <tbody>
                {devices.map((device) => (
                  <tr key={device.address}>
                    <td>{device.name || "ONVIF камера"}</td>
                    <td className="mono">{device.address}</td>
                    <td>{device.port}</td>
                    <td>
                      <button
                        type="button"
                        className="button secondary"
                        onClick={() => {
                          setProbeAddress(device.address);
                          setProbePort("8000");
                          setProbeResult(undefined);
                          setProbeError("");
                        }}
                      >
                        Проверить через HCNetSDK
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      <Panel title="HCNetSDK V40 — устройство и каналы">
        <p className="muted">
          Проверка выполняется через NET_DVR_Login_V40. Пароль используется только для входа и не возвращается в ответе API.
        </p>
        {probeError && <ErrorState message={probeError} />}
        <form className="network-profile-form" onSubmit={probeDevice}>
          <label>
            IP камеры / регистратора
            <input
              value={probeAddress}
              onChange={(event) => setProbeAddress(event.target.value)}
              placeholder="192.168.1.64"
              disabled={!canManage || probing}
              required
            />
          </label>
          <label>
            SDK порт
            <input
              type="number"
              min={1}
              max={65535}
              value={probePort}
              onChange={(event) => setProbePort(event.target.value)}
              disabled={!canManage || probing}
              required
            />
          </label>
          <label>
            Пользователь
            <input
              value={probeUsername}
              onChange={(event) => setProbeUsername(event.target.value)}
              disabled={!canManage || probing}
              required
            />
          </label>
          <label>
            Пароль
            <input
              type="password"
              value={probePassword}
              onChange={(event) => setProbePassword(event.target.value)}
              disabled={!canManage || probing}
            />
          </label>
          <button type="submit" className="button primary" disabled={!canManage || probing || !sdkReady}>
            {probing ? "Подключение…" : "Подключиться и прочитать устройство"}
          </button>
        </form>

        {probeResult?.device && (
          <>
            <div className="notice success">HCNetSDK V40: устройство доступно.</div>
            <dl className="details">
              <dt>Адрес</dt><dd className="mono">{probeResult.address}:{probeResult.port}</dd>
              <dt>Имя устройства</dt><dd>{probeResult.device.device_name || "—"}</dd>
              <dt>Модель / тип</dt><dd>{probeResult.device.device_type_name || `Type ${probeResult.device.device_type}`}</dd>
              <dt>Серийный номер</dt><dd className="mono">{probeResult.device.serial_number || "—"}</dd>
              <dt>Firmware</dt><dd>{probeResult.device.firmware || "—"}</dd>
              <dt>Аналоговые каналы</dt><dd>{probeResult.device.analog_channel_count}</dd>
              <dt>IP-каналы</dt><dd>{probeResult.device.ip_channel_count}</dd>
            </dl>
            {probeResult.device.channels.length > 0 && (
              <div className="table-wrap">
                <table>
                  <thead><tr><th>Канал</th><th>Тип</th></tr></thead>
                  <tbody>
                    {probeResult.device.channels.map((channel) => (
                      <tr key={`${channel.kind}-${channel.number}`}>
                        <td>{channel.number}</td>
                        <td>{channel.kind === "ip" ? "IP" : "Аналоговый"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </Panel>
    </div>
  );
}
