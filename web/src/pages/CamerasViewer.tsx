import {FormEvent, useCallback, useEffect, useMemo, useState} from "react";
import {api} from "../api/client";
import {ErrorState, LoadingState, Panel} from "../components/Panel";
import {useResource} from "../hooks/useResource";
import {PageHeading} from "./Dashboard";

type XMLLike = XMLDocument | Document;

type HikvisionPortInfo = {
  iDevicePort: number;
  iRtspPort: number;
  iHttpPort: number;
};

type HikvisionWebVideoCtrl = {
  I_InitPlugin(options: Record<string, unknown>): void;
  I_InsertOBJECTPlugin(containerId: string): Promise<void>;
  I_Login(ip: string, protocol: number, port: number, username: string, password: string, options: Record<string, unknown>): Promise<void>;
  I_Logout(deviceIdentify: string): Promise<void>;
  I_GetDevicePort(deviceIdentify: string): Promise<HikvisionPortInfo>;
  I_GetAnalogChannelInfo(deviceIdentify: string, options: Record<string, unknown>): Promise<XMLLike>;
  I_GetDigitalChannelInfo(deviceIdentify: string, options: Record<string, unknown>): Promise<XMLLike>;
  I_GetZeroChannelInfo(deviceIdentify: string, options: Record<string, unknown>): Promise<XMLLike>;
  I_StartRealPlay(deviceIdentify: string, options: Record<string, unknown>): Promise<void>;
  I_RecordSearch(deviceIdentify: string, channelId: number, startTime: string, endTime: string, options: Record<string, unknown>): Promise<XMLLike>;
  I_StartPlayback(deviceIdentify: string, options: Record<string, unknown>): Promise<void>;
  I_ReversePlayback(deviceIdentify: string, options: Record<string, unknown>): Promise<void>;
  I_Stop(options?: unknown): Promise<void>;
  I_StopAllPlay(): Promise<void>;
  I_Pause(options?: unknown): Promise<void>;
  I_Resume(options?: unknown): Promise<void>;
  I_Frame(options?: unknown): Promise<void>;
  I_PlaySlow(options?: unknown): Promise<void>;
  I_PlayFast(options?: unknown): Promise<void>;
  I_GetWindowStatus(index: number): unknown;
};

declare global {
  interface Window {
    WebVideoCtrl?: HikvisionWebVideoCtrl;
  }
}

type ViewerChannel = {
  id: number;
  name: string;
  kind: "analog" | "digital" | "zero";
  zero: boolean;
};

type ArchiveItem = {
  trackId: string;
  startTime: string;
  endTime: string;
  playbackURI: string;
  metadata: string;
};

let webSDKLoadPromise: Promise<void> | undefined;

function loadScriptOnce(src: string, id: string): Promise<void> {
  const existing = document.getElementById(id) as HTMLScriptElement | null;
  if (existing?.dataset.loaded === "true") return Promise.resolve();
  if (existing) {
    return new Promise((resolve, reject) => {
      existing.addEventListener("load", () => resolve(), {once: true});
      existing.addEventListener("error", () => reject(new Error(`Не удалось загрузить ${src}`)), {once: true});
    });
  }
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.id = id;
    script.src = src;
    script.async = false;
    script.onload = () => {
      script.dataset.loaded = "true";
      resolve();
    };
    script.onerror = () => reject(new Error(`Не удалось загрузить ${src}`));
    document.head.appendChild(script);
  });
}

function ensureWebSDKLoaded(): Promise<void> {
  if (window.WebVideoCtrl) return Promise.resolve();
  if (!webSDKLoadPromise) {
    webSDKLoadPromise = loadScriptOnce(api.cameraWebSDKAssetURL("jquery-1.7.1.min.js"), "hikvision-websdk-jquery")
      .then(() => loadScriptOnce(api.cameraWebSDKAssetURL("webVideoCtrl.js"), "hikvision-websdk-control"))
      .then(() => {
        if (!window.WebVideoCtrl) throw new Error("WebSDK V3.3.1 не инициализирован");
      });
  }
  return webSDKLoadPromise;
}

function childText(node: Element, tag: string): string {
  return node.getElementsByTagName(tag)[0]?.textContent?.trim() || "";
}

function xmlElements(xml: XMLLike, tag: string): Element[] {
  return Array.from(xml.getElementsByTagName(tag));
}

function parseChannels(analog?: XMLLike, digital?: XMLLike, zero?: XMLLike): ViewerChannel[] {
  const result: ViewerChannel[] = [];
  for (const [index, node] of (analog ? xmlElements(analog, "VideoInputChannel") : []).entries()) {
    const id = Number(childText(node, "id"));
    if (!Number.isFinite(id)) continue;
    result.push({
      id,
      name: childText(node, "name") || `Camera ${String(index + 1).padStart(2, "0")}`,
      kind: "analog",
      zero: false,
    });
  }
  for (const [index, node] of (digital ? xmlElements(digital, "InputProxyChannelStatus") : []).entries()) {
    if (childText(node, "online").toLowerCase() === "false") continue;
    const id = Number(childText(node, "id"));
    if (!Number.isFinite(id)) continue;
    result.push({
      id,
      name: childText(node, "name") || `IPCamera ${String(index + 1).padStart(2, "0")}`,
      kind: "digital",
      zero: false,
    });
  }
  for (const [index, node] of (zero ? xmlElements(zero, "ZeroVideoChannel") : []).entries()) {
    if (childText(node, "enabled").toLowerCase() !== "true") continue;
    const id = Number(childText(node, "id"));
    if (!Number.isFinite(id)) continue;
    result.push({
      id,
      name: childText(node, "name") || `Zero Channel ${String(index + 1).padStart(2, "0")}`,
      kind: "zero",
      zero: true,
    });
  }
  return result.sort((left, right) => left.zero === right.zero ? left.id - right.id : left.zero ? 1 : -1);
}

function toSDKTime(value: string): string {
  if (!value) return "";
  const normalized = value.replace("T", " ").replace(/Z$/, "");
  return normalized.length === 16 ? normalized + ":00" : normalized;
}

function toInputTime(date: Date): string {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function archiveTime(value: string): string {
  return value.replace("T", " ").replace(/Z$/, "");
}

function parseArchive(xml: XMLLike): {status: string; items: ArchiveItem[]} {
  const status = xml.getElementsByTagName("responseStatusStrg")[0]?.textContent?.trim() || "";
  const items = xmlElements(xml, "searchMatchItem").map((node) => ({
    trackId: childText(node, "trackID"),
    startTime: childText(node, "startTime"),
    endTime: childText(node, "endTime"),
    playbackURI: childText(node, "playbackURI"),
    metadata: childText(node, "metadataDescriptor"),
  }));
  return {status, items};
}

async function stopWindow(ctrl: HikvisionWebVideoCtrl): Promise<void> {
  if (!ctrl.I_GetWindowStatus(0)) return;
  try {
    await ctrl.I_Stop({iWndIndex: 0});
  } catch {
    // The vendor plugin may already have stopped the window after a stream error.
  }
}

export function CamerasViewerPage({revision, onNavigate}: {revision: number; onNavigate: (path: string) => void}) {
  const load = useCallback(() => api.camerasStatus(), []);
  const resource = useResource(load, revision);
  const [pluginState, setPluginState] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const [pluginError, setPluginError] = useState("");
  const [address, setAddress] = useState("");
  const [port, setPort] = useState("80");
  const [https, setHTTPS] = useState(false);
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [deviceIdentify, setDeviceIdentify] = useState("");
  const [rtspPort, setRTSPPort] = useState(554);
  const [channels, setChannels] = useState<ViewerChannel[]>([]);
  const [channelId, setChannelId] = useState<number>();
  const [streamType, setStreamType] = useState(1);
  const [connectionBusy, setConnectionBusy] = useState(false);
  const [viewerError, setViewerError] = useState("");
  const [mode, setMode] = useState<"live" | "archive">("live");
  const [playing, setPlaying] = useState("");
  const now = useMemo(() => new Date(), []);
  const [archiveStart, setArchiveStart] = useState(() => {
    const start = new Date(now);
    start.setHours(0, 0, 0, 0);
    return toInputTime(start);
  });
  const [archiveEnd, setArchiveEnd] = useState(() => toInputTime(now));
  const [archiveBusy, setArchiveBusy] = useState(false);
  const [records, setRecords] = useState<ArchiveItem[]>([]);
  const [selectedRecord, setSelectedRecord] = useState<ArchiveItem>();

  const webSDKAvailable = resource.data?.websdk.available === true;

  useEffect(() => {
    if (!webSDKAvailable) return;
    let cancelled = false;
    setPluginState("loading");
    setPluginError("");

    ensureWebSDKLoaded()
      .then(() => new Promise<void>((resolve, reject) => {
        const ctrl = window.WebVideoCtrl;
        if (!ctrl) {
          reject(new Error("WebSDK V3.3.1 не загружен"));
          return;
        }
        ctrl.I_InitPlugin({
          bWndFull: true,
          iWndowType: 1,
          cbSelWnd: () => undefined,
          cbDoubleClickWnd: () => undefined,
          cbEvent: (eventType: number) => {
            if (eventType === 2 && !cancelled) setPlaying("");
          },
          cbInitPluginComplete: () => {
            ctrl.I_InsertOBJECTPlugin("home-ai-hikvision-websdk-viewer").then(resolve, reject);
          },
        });
      }))
      .then(() => {
        if (!cancelled) setPluginState("ready");
      })
      .catch((reason) => {
        if (cancelled) return;
        setPluginState("error");
        setPluginError(reason instanceof Error ? reason.message : "HCWebSDKPlugin не запущен");
      });

    return () => {
      cancelled = true;
      const ctrl = window.WebVideoCtrl;
      if (ctrl) {
        void ctrl.I_StopAllPlay().catch(() => undefined);
        if (deviceIdentify) void ctrl.I_Logout(deviceIdentify).catch(() => undefined);
      }
    };
  }, [webSDKAvailable, deviceIdentify]);

  const connect = async (event: FormEvent) => {
    event.preventDefault();
    const ctrl = window.WebVideoCtrl;
    if (!ctrl || pluginState !== "ready" || connectionBusy) return;
    setConnectionBusy(true);
    setViewerError("");
    setPlaying("");
    try {
      if (deviceIdentify) {
        await ctrl.I_Logout(deviceIdentify).catch(() => undefined);
      }
      const numericPort = Number(port || (https ? "443" : "80"));
      await ctrl.I_Login(address.trim(), https ? 2 : 1, numericPort, username.trim(), password, {timeout: 5000});
      const identify = `${address.trim()}_${numericPort}`;
      setDeviceIdentify(identify);

      const [portsResult, analogResult, digitalResult, zeroResult] = await Promise.allSettled([
        ctrl.I_GetDevicePort(identify),
        ctrl.I_GetAnalogChannelInfo(identify, {}),
        ctrl.I_GetDigitalChannelInfo(identify, {}),
        ctrl.I_GetZeroChannelInfo(identify, {}),
      ]);
      if (portsResult.status === "fulfilled" && portsResult.value.iRtspPort > 0) {
        setRTSPPort(portsResult.value.iRtspPort);
      }
      const list = parseChannels(
        analogResult.status === "fulfilled" ? analogResult.value : undefined,
        digitalResult.status === "fulfilled" ? digitalResult.value : undefined,
        zeroResult.status === "fulfilled" ? zeroResult.value : undefined,
      );
      setChannels(list);
      setChannelId(list[0]?.id);
      if (!list.length) {
        setViewerError("Устройство подключено, но WebSDK не вернул доступные каналы.");
      }
    } catch (reason) {
      setDeviceIdentify("");
      setChannels([]);
      setChannelId(undefined);
      setViewerError(reason instanceof Error ? reason.message : "WebSDK login failed");
    } finally {
      setConnectionBusy(false);
    }
  };

  const startLive = async () => {
    const ctrl = window.WebVideoCtrl;
    const channel = channels.find((item) => item.id === channelId);
    if (!ctrl || !deviceIdentify || !channel || connectionBusy) return;
    setViewerError("");
    try {
      await stopWindow(ctrl);
      await ctrl.I_StartRealPlay(deviceIdentify, {
        iWndIndex: 0,
        iStreamType: streamType,
        iChannelID: channel.id,
        bZeroChannel: channel.zero,
      });
      setMode("live");
      setPlaying(`Live · ${channel.name} · ${streamType === 1 ? "Main" : "Sub"}`);
    } catch (reason) {
      setViewerError(reason instanceof Error ? reason.message : "Не удалось запустить Live");
    }
  };

  const stop = async () => {
    const ctrl = window.WebVideoCtrl;
    if (!ctrl) return;
    await stopWindow(ctrl);
    setPlaying("");
  };

  const searchArchive = async (event: FormEvent) => {
    event.preventDefault();
    const ctrl = window.WebVideoCtrl;
    const channel = channels.find((item) => item.id === channelId);
    if (!ctrl || !deviceIdentify || !channel || channel.zero || archiveBusy) return;
    setArchiveBusy(true);
    setViewerError("");
    setRecords([]);
    setSelectedRecord(undefined);
    try {
      const all: ArchiveItem[] = [];
      let position = 0;
      for (let page = 0; page < 10; page++) {
        const xml = await ctrl.I_RecordSearch(
          deviceIdentify,
          channel.id,
          toSDKTime(archiveStart),
          toSDKTime(archiveEnd),
          {iStreamType: streamType, iSearchPos: position},
        );
        const parsed = parseArchive(xml);
        all.push(...parsed.items);
        if (parsed.status !== "MORE" || parsed.items.length === 0) break;
        position += parsed.items.length;
      }
      setRecords(all);
      setSelectedRecord(all[0]);
      setMode("archive");
      if (!all.length) setViewerError("За выбранный период записей не найдено.");
    } catch (reason) {
      setViewerError(reason instanceof Error ? reason.message : "Ошибка поиска архива");
    } finally {
      setArchiveBusy(false);
    }
  };

  const playRecord = async (reverse = false) => {
    const ctrl = window.WebVideoCtrl;
    const channel = channels.find((item) => item.id === channelId);
    if (!ctrl || !deviceIdentify || !channel || !selectedRecord) return;
    setViewerError("");
    try {
      await stopWindow(ctrl);
      const options = {
        iWndIndex: 0,
        iRtspPort: rtspPort,
        iStreamType: streamType,
        iChannelID: channel.id,
        szStartTime: archiveTime(selectedRecord.startTime),
        szEndTime: archiveTime(selectedRecord.endTime),
      };
      if (reverse) {
        await ctrl.I_ReversePlayback(deviceIdentify, options);
      } else {
        await ctrl.I_StartPlayback(deviceIdentify, options);
      }
      setMode("archive");
      setPlaying(`${reverse ? "Reverse archive" : "Archive"} · ${channel.name} · ${archiveTime(selectedRecord.startTime)}`);
    } catch (reason) {
      setViewerError(reason instanceof Error ? reason.message : "Не удалось открыть архив");
    }
  };

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  return (
    <div className="page">
      <PageHeading title="Просмотр камер" subtitle="Оригинальный Hikvision WebSDK V3.3.1: Live и архив на отдельной странице." />
      {resource.error && <ErrorState message={resource.error} />}

      {!webSDKAvailable ? (
        <Panel title="WebSDK не установлен">
          <div className="notice warning">
            Для этой страницы требуется оригинальный WebSDK V3.3.1 ZIP. Установите его в настройках Cameras.
          </div>
          <button type="button" className="button primary" onClick={() => onNavigate("/modules/cameras")}>
            Открыть настройки Cameras
          </button>
        </Panel>
      ) : (
        <>
          <div className="camera-viewer-status">
            <span>WebSDK: <strong>V{resource.data?.websdk.version || "3.3.1"}</strong></span>
            <span>Plugin: <strong>{pluginState === "ready" ? "готов" : pluginState === "loading" ? "инициализация" : pluginState === "error" ? "ошибка" : "ожидание"}</strong></span>
            <span>RTSP: <strong>{rtspPort}</strong></span>
            {playing && <span>Сейчас: <strong>{playing}</strong></span>}
          </div>

          {pluginError && (
            <Panel title="HCWebSDKPlugin требуется на этом компьютере">
              <ErrorState message={pluginError} />
              <p className="muted">
                WebSDK V3.3.1 использует локальный Windows HCWebSDKPlugin для Live и Playback. Установите оригинальный plugin из присланного SDK и обновите страницу.
              </p>
              <a className="button primary" href={api.cameraWebSDKAssetURL("HCWebSDKPlugin.exe")}>
                Скачать HCWebSDKPlugin.exe
              </a>
            </Panel>
          )}

          <div className="camera-viewer-shell">
            <div className="camera-viewer-sidebar">
              <Panel title="Подключение">
                <form className="network-profile-form" onSubmit={connect}>
                  <label>
                    IP устройства
                    <input value={address} onChange={(event) => setAddress(event.target.value)} placeholder="192.168.1.64" required />
                  </label>
                  <label>
                    Web / ISAPI порт
                    <input type="number" min={1} max={65535} value={port} onChange={(event) => setPort(event.target.value)} required />
                  </label>
                  <label>
                    <input
                      type="checkbox"
                      checked={https}
                      onChange={(event) => {
                        const enabled = event.target.checked;
                        setHTTPS(enabled);
                        if (port === "80" || port === "443") setPort(enabled ? "443" : "80");
                      }}
                    />
                    HTTPS
                  </label>
                  <label>
                    Логин
                    <input value={username} onChange={(event) => setUsername(event.target.value)} required />
                  </label>
                  <label>
                    Пароль
                    <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
                  </label>
                  <button type="submit" className="button primary" disabled={pluginState !== "ready" || connectionBusy}>
                    {connectionBusy ? "Подключение…" : deviceIdentify ? "Переподключить" : "Подключить"}
                  </button>
                </form>
              </Panel>

              <Panel title="Канал">
                <label>
                  Камера / канал
                  <select value={channelId ?? ""} onChange={(event) => setChannelId(Number(event.target.value))} disabled={!channels.length}>
                    {!channels.length && <option value="">Нет каналов</option>}
                    {channels.map((channel) => (
                      <option key={`${channel.kind}-${channel.id}`} value={channel.id}>
                        {channel.name} · {channel.kind === "digital" ? "IP" : channel.kind === "zero" ? "Zero" : "Analog"} · {channel.id}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Поток
                  <select value={streamType} onChange={(event) => setStreamType(Number(event.target.value))}>
                    <option value={1}>Main</option>
                    <option value={2}>Sub</option>
                    <option value={3}>Third</option>
                  </select>
                </label>
              </Panel>

              <Panel title="Режим">
                <div className="camera-viewer-tabs">
                  <button type="button" className={`button secondary${mode === "live" ? " active" : ""}`} onClick={() => setMode("live")}>
                    Live
                  </button>
                  <button type="button" className={`button secondary${mode === "archive" ? " active" : ""}`} onClick={() => setMode("archive")}>
                    Архив
                  </button>
                </div>
                {mode === "live" ? (
                  <div className="camera-viewer-controls">
                    <button type="button" className="button primary" onClick={startLive} disabled={!deviceIdentify || !channelId}>
                      ▶ Live
                    </button>
                    <button type="button" className="button secondary" onClick={stop}>■ Stop</button>
                  </div>
                ) : (
                  <form className="network-profile-form" onSubmit={searchArchive}>
                    <label>
                      С
                      <input type="datetime-local" value={archiveStart} onChange={(event) => setArchiveStart(event.target.value)} />
                    </label>
                    <label>
                      По
                      <input type="datetime-local" value={archiveEnd} onChange={(event) => setArchiveEnd(event.target.value)} />
                    </label>
                    <button type="submit" className="button primary" disabled={!deviceIdentify || !channelId || archiveBusy}>
                      {archiveBusy ? "Поиск…" : "Найти записи"}
                    </button>
                  </form>
                )}
              </Panel>
            </div>

            <div style={{display: "grid", gap: 16}}>
              {viewerError && <ErrorState message={viewerError} />}
              <Panel title={mode === "live" ? "Live" : "Архив"}>
                <div id="home-ai-hikvision-websdk-viewer" className="camera-viewer-stage" />
                <div className="camera-viewer-controls" style={{marginTop: 12}}>
                  <button type="button" className="button secondary" onClick={stop}>Stop</button>
                  {mode === "archive" && (
                    <>
                      <button type="button" className="button secondary" onClick={() => window.WebVideoCtrl?.I_Pause({iWndIndex: 0})}>Pause</button>
                      <button type="button" className="button secondary" onClick={() => window.WebVideoCtrl?.I_Resume({iWndIndex: 0})}>Resume</button>
                      <button type="button" className="button secondary" onClick={() => window.WebVideoCtrl?.I_Frame({iWndIndex: 0})}>Frame</button>
                      <button type="button" className="button secondary" onClick={() => window.WebVideoCtrl?.I_PlaySlow({iWndIndex: 0})}>Slow</button>
                      <button type="button" className="button secondary" onClick={() => window.WebVideoCtrl?.I_PlayFast({iWndIndex: 0})}>Fast</button>
                    </>
                  )}
                </div>
              </Panel>

              {mode === "archive" && (
                <Panel title={`Найденные записи · ${records.length}`}>
                  {records.length === 0 ? (
                    <p className="muted">Выберите период и нажмите «Найти записи».</p>
                  ) : (
                    <>
                      <div className="camera-viewer-records table-wrap">
                        <table>
                          <thead><tr><th>Начало</th><th>Конец</th><th>Track</th><th>Тип</th></tr></thead>
                          <tbody>
                            {records.map((record, index) => {
                              const selected = record === selectedRecord;
                              return (
                                <tr key={`${record.trackId}-${record.startTime}-${index}`}>
                                  <td>
                                    <button type="button" className="camera-viewer-record" onClick={() => setSelectedRecord(record)}>
                                      <strong>{selected ? "▶ " : ""}{archiveTime(record.startTime)}</strong>
                                    </button>
                                  </td>
                                  <td>{archiveTime(record.endTime)}</td>
                                  <td className="mono">{record.trackId || "—"}</td>
                                  <td>{record.metadata || "video"}</td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </div>
                      <div className="camera-viewer-controls" style={{marginTop: 12}}>
                        <button type="button" className="button primary" onClick={() => playRecord(false)} disabled={!selectedRecord}>
                          ▶ Воспроизвести
                        </button>
                        <button type="button" className="button secondary" onClick={() => playRecord(true)} disabled={!selectedRecord}>
                          ◀ Reverse
                        </button>
                      </div>
                    </>
                  )}
                </Panel>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
