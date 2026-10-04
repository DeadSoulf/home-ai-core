import {FormEvent, useCallback, useState} from "react";
import {api} from "../api/client";
import type {Actor, CameraSDKLoginResult, CameraWebSDKFunction, CameraWebSDKProbeResult, CameraWebSDKRawResponse} from "../api/types";
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
  const [webAddress, setWebAddress] = useState("");
  const [webPort, setWebPort] = useState("80");
  const [webHTTPS, setWebHTTPS] = useState(false);
  const [webUsername, setWebUsername] = useState("admin");
  const [webPassword, setWebPassword] = useState("");
  const [webProbing, setWebProbing] = useState(false);
  const [webError, setWebError] = useState("");
  const [webResult, setWebResult] = useState<CameraWebSDKProbeResult>();
  const [webFunctions, setWebFunctions] = useState<CameraWebSDKFunction[]>([]);
  const [webFunctionsLoading, setWebFunctionsLoading] = useState(false);
  const [rawMethod, setRawMethod] = useState<"GET" | "POST" | "PUT" | "DELETE">("GET");
  const [rawPath, setRawPath] = useState("/ISAPI/System/capabilities");
  const [rawContentType, setRawContentType] = useState("application/xml");
  const [rawBody, setRawBody] = useState("");
  const [rawBusy, setRawBusy] = useState(false);
  const [rawError, setRawError] = useState("");
  const [rawResult, setRawResult] = useState<CameraWebSDKRawResponse>();

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

  const probeWebSDK = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage || webProbing) return;
    setWebProbing(true);
    setWebError("");
    setWebResult(undefined);
    try {
      const result = await api.probeCameraWebSDK({
        address: webAddress.trim(),
        port: Number(webPort || (webHTTPS ? "443" : "80")),
        https: webHTTPS,
        username: webUsername.trim(),
        password: webPassword,
      });
      setWebResult(result);
      setProbeAddress(result.address);
      if (result.ports.device_port) setProbePort(String(result.ports.device_port));
      setProbeUsername(webUsername.trim());
      setProbePassword(webPassword);
    } catch (reason) {
      setWebError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setWebProbing(false);
    }
  };

  const loadWebSDKFunctions = async () => {
    if (webFunctionsLoading) return;
    setWebFunctionsLoading(true);
    try {
      setWebFunctions(await api.cameraWebSDKFunctions());
    } catch (reason) {
      setWebError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setWebFunctionsLoading(false);
    }
  };

  const sendRawWebSDKRequest = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage || rawBusy) return;
    setRawBusy(true);
    setRawError("");
    setRawResult(undefined);
    try {
      const result = await api.sendCameraWebSDKRequest({
        address: webAddress.trim(),
        port: Number(webPort || (webHTTPS ? "443" : "80")),
        https: webHTTPS,
        username: webUsername.trim(),
        password: webPassword,
        method: rawMethod,
        path: rawPath.trim(),
        content_type: rawBody ? rawContentType.trim() || "application/xml" : undefined,
        body: rawBody || undefined,
      });
      setRawResult(result);
    } catch (reason) {
      setRawError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setRawBusy(false);
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
        <button type="button" className="button primary" onClick={discover} disabled={searching}>
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
                          setWebAddress(device.address);
                          setWebPort(String(device.port || 80));
                          setWebResult(undefined);
                          setWebError("");
                        }}
                      >
                        Открыть через WebSDK
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      <Panel title="WebSDK V3.3.1 / ISAPI — устройство и каналы">
        <p className="muted">
          HOME AI использует официальный Hikvision WebSDK workflow на сервере Core: userCheck, deviceInfo,
          analog/digital channels, service ports и streaming channels. Windows HCWebSDKPlugin для этого не требуется.
        </p>
        {webError && <ErrorState message={webError} />}
        <form className="network-profile-form" onSubmit={probeWebSDK}>
          <label>
            IP камеры / регистратора
            <input
              value={webAddress}
              onChange={(event) => setWebAddress(event.target.value)}
              placeholder="192.168.1.64"
              disabled={!canManage || webProbing}
              required
            />
          </label>
          <label>
            Web / ISAPI порт
            <input
              type="number"
              min={1}
              max={65535}
              value={webPort}
              onChange={(event) => setWebPort(event.target.value)}
              disabled={!canManage || webProbing}
              required
            />
          </label>
          <label>
            <input
              type="checkbox"
              checked={webHTTPS}
              onChange={(event) => {
                const enabled = event.target.checked;
                setWebHTTPS(enabled);
                if (webPort === "80" || webPort === "443") setWebPort(enabled ? "443" : "80");
              }}
              disabled={!canManage || webProbing}
            />
            HTTPS
          </label>
          <label>
            Пользователь
            <input
              value={webUsername}
              onChange={(event) => setWebUsername(event.target.value)}
              disabled={!canManage || webProbing}
              required
            />
          </label>
          <label>
            Пароль
            <input
              type="password"
              value={webPassword}
              onChange={(event) => setWebPassword(event.target.value)}
              disabled={!canManage || webProbing}
            />
          </label>
          <button type="submit" className="button primary" disabled={!canManage || webProbing}>
            {webProbing ? "Чтение WebSDK…" : "Подключиться через WebSDK"}
          </button>
        </form>

        {webResult && (
          <>
            <div className="notice success">WebSDK/ISAPI: устройство доступно.</div>
            <dl className="details">
              <dt>Адрес</dt><dd className="mono">{webResult.https ? "https" : "http"}://{webResult.address}:{webResult.port}</dd>
              <dt>Имя устройства</dt><dd>{webResult.device.device_name || "—"}</dd>
              <dt>Модель</dt><dd>{webResult.device.model || "—"}</dd>
              <dt>Тип</dt><dd>{webResult.device.device_type || "—"}</dd>
              <dt>Серийный номер</dt><dd className="mono">{webResult.device.serial_number || "—"}</dd>
              <dt>Firmware</dt><dd>{webResult.device.firmware_version || "—"}</dd>
              <dt>MAC</dt><dd className="mono">{webResult.device.mac_address || "—"}</dd>
              <dt>HTTP</dt><dd>{webResult.ports.http_port || "—"}</dd>
              <dt>RTSP</dt><dd>{webResult.ports.rtsp_port || "—"}</dd>
              <dt>HCNetSDK</dt><dd>{webResult.ports.device_port || "—"}</dd>
              <dt>Streaming profiles</dt><dd>{webResult.streams.length}</dd>
            </dl>
            {webResult.warnings?.map((warning) => (
              <div className="notice warning" key={warning}>{warning}</div>
            ))}
            {webResult.channels.length > 0 && (
              <div className="table-wrap">
                <table>
                  <thead><tr><th>ID</th><th>Тип</th><th>Имя</th><th>Состояние</th><th>Источник</th></tr></thead>
                  <tbody>
                    {webResult.channels.map((channel) => (
                      <tr key={`${channel.kind}-${channel.id}`}>
                        <td>{channel.id}</td>
                        <td>{channel.kind === "digital" ? "IP" : "Аналоговый"}</td>
                        <td>{channel.name || "—"}</td>
                        <td>{channel.online === undefined ? "—" : channel.online ? "Online" : "Offline"}</td>
                        <td className="mono">
                          {channel.ip_address ? `${channel.ip_address}${channel.manage_port ? `:${channel.manage_port}` : ""}` : "—"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </Panel>

      <Panel title="WebSDK V3.3.1 — полный API surface">
        <p className="muted">
          В архиве WebSDK V3.3.1 найдено 79 публичных I_* функций. HOME AI сохраняет весь surface:
          HTTP/ISAPI операции выполняет Core, media-функции сопоставлены с HCNetSDK, а оконные/plugin-функции —
          с обычным Web-интерфейсом без HCWebSDKPlugin.exe.
        </p>
        <button type="button" className="button secondary" onClick={loadWebSDKFunctions} disabled={webFunctionsLoading}>
          {webFunctionsLoading ? "Загрузка…" : webFunctions.length ? "Обновить список функций" : "Показать все 79 функций"}
        </button>
        {webFunctions.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead><tr><th>Функция</th><th>Группа</th><th>Backend</th><th>Статус</th></tr></thead>
              <tbody>
                {webFunctions.map((item) => (
                  <tr key={item.name}>
                    <td className="mono">{item.name}</td>
                    <td>{item.category}</td>
                    <td className="mono">{item.backend}</td>
                    <td>{item.status === "implemented" ? "Готово" : "Привязано к backend"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <h3>Расширенный WebSDK / ISAPI запрос</h3>
        <p className="muted">
          Это серверный эквивалент I_SendHTTPRequest. Разрешены только Hikvision /ISAPI, /SDK и ZeroStreaming PSIA
          на private/link-local IP; прокси и redirects отключены.
        </p>
        {rawError && <ErrorState message={rawError} />}
        <form className="network-profile-form" onSubmit={sendRawWebSDKRequest}>
          <label>
            Метод
            <select value={rawMethod} onChange={(event) => setRawMethod(event.target.value as typeof rawMethod)} disabled={!canManage || rawBusy}>
              <option value="GET">GET</option>
              <option value="POST">POST</option>
              <option value="PUT">PUT</option>
              <option value="DELETE">DELETE</option>
            </select>
          </label>
          <label>
            API path
            <input
              className="mono"
              value={rawPath}
              onChange={(event) => setRawPath(event.target.value)}
              placeholder="/ISAPI/System/deviceInfo"
              disabled={!canManage || rawBusy}
              required
            />
          </label>
          <label>
            Content-Type
            <input
              className="mono"
              value={rawContentType}
              onChange={(event) => setRawContentType(event.target.value)}
              disabled={!canManage || rawBusy}
            />
          </label>
          <label>
            XML / JSON body
            <textarea
              className="mono"
              value={rawBody}
              onChange={(event) => setRawBody(event.target.value)}
              rows={8}
              disabled={!canManage || rawBusy}
            />
          </label>
          <button
            type="submit"
            className="button primary"
            disabled={!canManage || rawBusy || !webAddress.trim() || !webUsername.trim() || !rawPath.trim()}
          >
            {rawBusy ? "Выполнение…" : "Выполнить WebSDK запрос"}
          </button>
        </form>
        {rawResult && (
          <>
            <div className="notice success">
              HTTP {rawResult.status} · {rawResult.content_type || "unknown content-type"} · {rawResult.binary ? "binary" : "text"}
            </div>
            <pre className="mono" style={{whiteSpace: "pre-wrap", overflowWrap: "anywhere"}}>
              {rawResult.body || (rawResult.body_base64 ? `base64: ${rawResult.body_base64.slice(0, 4096)}${rawResult.body_base64.length > 4096 ? "…" : ""}` : "Пустой ответ")}
            </pre>
          </>
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
