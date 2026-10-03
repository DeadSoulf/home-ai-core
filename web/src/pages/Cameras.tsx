import {FormEvent, useCallback, useMemo, useState} from "react";
import {api} from "../api/client";
import type {
  Actor,
  NVRCamera,
  NVRCameraInput,
  NVRProbe,
  NVRStorageTarget,
  NVRONVIFDevice,
  NVRONVIFProfile,
} from "../api/types";
import {EmptyState, ErrorState, LoadingState, Panel} from "../components/Panel";
import {useResource} from "../hooks/useResource";
import {useI18n} from "../i18n";
import {PageHeading, Status} from "./Dashboard";

type CameraEditor = {
  name: string;
  address: string;
  substreamAddress: string;
  username: string;
  password: string;
  transport: "tcp" | "udp";
  recordingMode: "off" | "continuous" | "motion";
  audioEnabled: boolean;
  enabled: boolean;
  clearCredentials: boolean;
  clearSubstream: boolean;
};

const emptyEditor = (): CameraEditor => ({
  name: "",
  address: "",
  substreamAddress: "",
  username: "",
  password: "",
  transport: "tcp",
  recordingMode: "off",
  audioEnabled: false,
  enabled: true,
  clearCredentials: false,
  clearSubstream: false,
});

function cameraInput(editor: CameraEditor): NVRCameraInput {
  return {
    name: editor.name.trim(),
    address: editor.address.trim(),
    substream_address: editor.substreamAddress.trim() || undefined,
    username: editor.username.trim() || undefined,
    password: editor.password || undefined,
    transport: editor.transport,
    recording_mode: editor.recordingMode,
    audio_enabled: editor.audioEnabled,
    enabled: editor.enabled,
    clear_credentials: editor.clearCredentials,
    clear_substream: editor.clearSubstream,
  };
}

function probeFingerprint(editor: CameraEditor): string {
  return [
    editor.address.trim(),
    editor.substreamAddress.trim(),
    editor.username.trim(),
    editor.password,
    editor.transport,
  ].join("\u0000");
}

function hasCameraManage(actor: Actor, cameraID: string): boolean {
  if (actor.permissions.includes("camera.manage")) return true;
  return (actor.resource_permissions || []).some((scope) =>
    scope.permission === "camera.manage" &&
    scope.resource_type === "camera" &&
    scope.resource_id === cameraID
  );
}

function hasCameraLive(actor: Actor, cameraID: string): boolean {
  if (actor.permissions.includes("camera.live")) return true;
  return (actor.resource_permissions || []).some((scope) =>
    scope.permission === "camera.live" &&
    scope.resource_type === "camera" &&
    scope.resource_id === cameraID
  );
}

function formatStorageBytes(value?: number): string {
  if (!value || value <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = value;
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024;
    index++;
  }
  return `${amount.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function probeText(probe: NVRProbe): string {
  const size = probe.width > 0 && probe.height > 0 ? `${probe.width}×${probe.height}` : "—";
  const fps = probe.fps > 0 ? `${probe.fps.toFixed(2)} FPS` : "—";
  const bitrate = probe.bitrate_bps > 0 ? `${(probe.bitrate_bps / 1_000_000).toFixed(2)} Mbps` : "—";
  return `${probe.codec.toUpperCase()} · ${size} · ${fps} · ${bitrate}`;
}

export function CamerasPage({revision, actor}: {revision: number; actor: Actor}) {
  const {t, date} = useI18n();
  const storageManage = actor.permissions.includes("nvr.storage.manage");
  const runtimeManage = actor.permissions.includes("nvr.settings.manage");
  const load = useCallback(async () => {
    const [status, cameras, storage] = await Promise.all([
      api.nvrStatus(),
      api.nvrCameras(),
      storageManage ? api.nvrStorage() : Promise.resolve([] as NVRStorageTarget[]),
    ]);
    return {status, cameras, storage};
  }, [storageManage]);
  const resource = useResource(load, revision);

  const [editorOpen, setEditorOpen] = useState(false);
  const [editingID, setEditingID] = useState<string>();
  const [editor, setEditor] = useState<CameraEditor>(emptyEditor);
  const [busy, setBusy] = useState("");
  const [formError, setFormError] = useState("");
  const [storageMessage, setStorageMessage] = useState("");
  const [runtimeMessage, setRuntimeMessage] = useState("");
  const [probe, setProbe] = useState<NVRProbe>();
  const [testedFingerprint, setTestedFingerprint] = useState("");
  const [liveIDs, setLiveIDs] = useState<string[]>([]);
  const [liveErrors, setLiveErrors] = useState<Record<string, boolean>>({});
  const [onvifOpen, setONVIFOpen] = useState(false);
  const [onvifDevices, setONVIFDevices] = useState<NVRONVIFDevice[]>([]);
  const [onvifDevice, setONVIFDevice] = useState<NVRONVIFDevice>();
  const [onvifProfiles, setONVIFProfiles] = useState<NVRONVIFProfile[]>([]);
  const [onvifName, setONVIFName] = useState("");
  const [onvifUsername, setONVIFUsername] = useState("");
  const [onvifPassword, setONVIFPassword] = useState("");
  const [onvifMainToken, setONVIFMainToken] = useState("");
  const [onvifSubToken, setONVIFSubToken] = useState("");
  const [onvifTransport, setONVIFTransport] = useState<"tcp" | "udp">("tcp");
  const [onvifRecordingMode, setONVIFRecordingMode] = useState<"off" | "continuous" | "motion">("off");
  const [onvifAudioEnabled, setONVIFAudioEnabled] = useState(false);
  const [storageDevice, setStorageDevice] = useState("");
  const [storageReserve, setStorageReserve] = useState(0);

  const globalManage = actor.permissions.includes("camera.manage");
  const currentFingerprint = useMemo(() => probeFingerprint(editor), [editor]);
  const createProbeValid = Boolean(probe && testedFingerprint === currentFingerprint);

  const resetEditor = () => {
    setEditorOpen(false);
    setEditingID(undefined);
    setEditor(emptyEditor());
    setProbe(undefined);
    setTestedFingerprint("");
    setFormError("");
    setBusy("");
  };

  const openCreate = () => {
    resetEditor();
    setEditorOpen(true);
  };

  const resetONVIF = () => {
    setONVIFOpen(false);
    setONVIFDevices([]);
    setONVIFDevice(undefined);
    setONVIFProfiles([]);
    setONVIFName("");
    setONVIFUsername("");
    setONVIFPassword("");
    setONVIFMainToken("");
    setONVIFSubToken("");
    setONVIFTransport("tcp");
    setONVIFRecordingMode("off");
    setONVIFAudioEnabled(false);
  };

  const discoverONVIF = async () => {
    setBusy("onvif:discover");
    setFormError("");
    try {
      const devices = await api.discoverONVIF();
      setONVIFDevices(devices);
      setONVIFDevice(undefined);
      setONVIFProfiles([]);
      setONVIFOpen(true);
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  };

  const chooseONVIFDevice = (device: NVRONVIFDevice) => {
    setONVIFDevice(device);
    setONVIFName(device.name);
    setONVIFProfiles([]);
    setONVIFMainToken("");
    setONVIFSubToken("");
    setONVIFUsername("");
    setONVIFPassword("");
  };

  const loadONVIFProfiles = async () => {
    if (!onvifDevice) return;
    setBusy("onvif:profiles");
    setFormError("");
    try {
      const profiles = await api.onvifProfiles({
        address: onvifDevice.address,
        username: onvifUsername.trim() || undefined,
        password: onvifPassword || undefined,
      });
      setONVIFProfiles(profiles);
      setONVIFMainToken(profiles[0]?.token || "");
      setONVIFSubToken(profiles.length > 1 ? profiles[profiles.length - 1].token : "");
      setONVIFAudioEnabled(Boolean(profiles[0]?.has_audio));
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
      setONVIFProfiles([]);
      setONVIFMainToken("");
      setONVIFSubToken("");
    } finally {
      setBusy("");
    }
  };

  const importONVIF = async (event: FormEvent) => {
    event.preventDefault();
    if (!onvifDevice || !onvifMainToken) return;
    setBusy("onvif:import");
    setFormError("");
    try {
      await api.importONVIFCamera({
        name: onvifName.trim(),
        address: onvifDevice.address,
        username: onvifUsername.trim() || undefined,
        password: onvifPassword || undefined,
        main_profile_token: onvifMainToken,
        sub_profile_token: onvifSubToken || undefined,
        transport: onvifTransport,
        recording_mode: onvifRecordingMode,
        audio_enabled: onvifAudioEnabled,
      });
      resetONVIF();
      resource.reload();
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  };

  const openEdit = async (camera: NVRCamera) => {
    setBusy("load:" + camera.id);
    setFormError("");
    try {
      const config = await api.nvrCamera(camera.id);
      setEditingID(camera.id);
      setEditor({
        name: config.name,
        address: config.address,
        substreamAddress: config.substream_address || "",
        username: "",
        password: "",
        transport: config.transport,
        recordingMode: config.recording_mode,
        audioEnabled: config.audio_enabled,
        enabled: config.enabled,
        clearCredentials: false,
        clearSubstream: false,
      });
      setProbe(undefined);
      setTestedFingerprint("");
      setEditorOpen(true);
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  };

  const testConnection = async () => {
    setBusy("test");
    setFormError("");
    setProbe(undefined);
    try {
      const result = editingID
        ? await api.testExistingNVRCamera(editingID)
        : await api.testNVRCamera(cameraInput(editor));
      setProbe(result);
      setTestedFingerprint(currentFingerprint);
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
      setTestedFingerprint("");
    } finally {
      setBusy("");
    }
  };

  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!editingID && !createProbeValid) {
      setFormError(t("nvrTestBeforeSave"));
      return;
    }
    setBusy("save");
    setFormError("");
    try {
      if (editingID) {
        const result = await api.updateNVRCamera(editingID, cameraInput(editor));
        setProbe(result.probe);
      } else {
        const result = await api.createNVRCamera(cameraInput(editor));
        setProbe(result.probe);
      }
      resetEditor();
      resource.reload();
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  };

  const deleteCamera = async (camera: NVRCamera) => {
    if (!window.confirm(t("nvrDeleteConfirm").replace("{camera}", camera.name))) return;
    setBusy("delete:" + camera.id);
    setFormError("");
    try {
      await api.deleteNVRCamera(camera.id);
      if (editingID === camera.id) resetEditor();
      setLiveIDs((items) => items.filter((id) => id !== camera.id));
      setLiveErrors((items) => {
        const next = {...items};
        delete next[camera.id];
        return next;
      });
      resource.reload();
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  };

  const testSavedCamera = async (camera: NVRCamera) => {
    setBusy("test:" + camera.id);
    setFormError("");
    try {
      const result = await api.testExistingNVRCamera(camera.id);
      window.alert(t("nvrTestSuccess") + "\n" + probeText(result));
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  };

  const toggleLive = (camera: NVRCamera) => {
    setLiveErrors((items) => {
      const next = {...items};
      delete next[camera.id];
      return next;
    });
    setLiveIDs((items) =>
      items.includes(camera.id)
        ? items.filter((id) => id !== camera.id)
        : [...items, camera.id],
    );
  };

  const openAllLive = (cameras: NVRCamera[]) => {
    setLiveErrors({});
    setLiveIDs(cameras.filter((camera) =>
      camera.enabled &&
      camera.runtime.state !== "offline" &&
      hasCameraLive(actor, camera.id)
    ).map((camera) => camera.id));
  };

  const closeAllLive = () => {
    setLiveIDs([]);
    setLiveErrors({});
  };

  const installRuntime = async () => {
    setBusy("runtime:install");
    setFormError("");
    setRuntimeMessage("");
    try {
      const result = await api.installNVRRuntime();
      setRuntimeMessage(result.message || t("nvrRuntimeInstalled"));
      resource.reload();
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  };

  const saveStorage = async () => {
    const active = resource.data?.storage.find((target) => target.active);
    const fallback = resource.data?.storage.find((target) => target.ready);
    const device = storageDevice || active?.device || fallback?.device || "";
    const reservePercent = storageReserve || active?.reserve_percent || 5;
    if (!device) return;

    setBusy("storage");
    setFormError("");
    setStorageMessage("");
    try {
      const target = await api.setNVRStorage(device, reservePercent);
      setStorageDevice(target.device);
      setStorageReserve(target.reserve_percent);
      setStorageMessage(t("nvrStorageSaved"));
      resource.reload();
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  };

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  const data = resource.data!;
  const activeStorage = data.storage.find((target) => target.active);
  const defaultStorage = activeStorage || data.storage.find((target) => target.ready);
  const selectedStorageDevice = storageDevice || defaultStorage?.device || "";
  const selectedStorage = data.storage.find((target) => target.device === selectedStorageDevice) || defaultStorage;
  const selectedStorageReserve = storageReserve || activeStorage?.reserve_percent || 5;
  const selectedStorageUsed = selectedStorage?.free_known && selectedStorage.size_bytes !== undefined && selectedStorage.free_bytes !== undefined
    ? Math.max(0, selectedStorage.size_bytes - selectedStorage.free_bytes)
    : undefined;

  return (
    <div className="page">
      <PageHeading title={t("cameras")} subtitle={t("camerasSubtitle")} />

      <div className="metric-grid overview-metrics">
        <div className="metric">
          <span>{t("cameras")}</span>
          <strong>{data.status.camera_count}</strong>
          <small>{t("nvrFoundationStage")}: {data.status.foundation_stage.toUpperCase()}</small>
        </div>
        <div className="metric">
          <span>{t("nvrOnline")} / {t("nvrOffline")}</span>
          <strong>{data.status.online_count} / {data.status.offline_count}</strong>
          <small>{t("nvrMediaRuntimeHint")}</small>
        </div>
        <div className="metric">
          <span>{t("nvrSupervisor")}</span>
          <strong>{data.status.supervisor_running ? t("nvrSupervisorRunning") : t("nvrSupervisorStopped")}</strong>
          <small>{data.status.media_runtime_ready ? "ffprobe OK" : "ffprobe —"}</small>
        </div>
        <div className="metric">
          <span>{t("nvrCredentialStore")}</span>
          <strong>{data.status.secret_store_ready ? t("available") : t("nvrNotYet")}</strong>
          <small>{t("nvrCredentialStoreHint")}</small>
        </div>
        <div className="metric">
          <span>{t("nvrLiveRuntime")}</span>
          <strong>{data.status.live_runtime_ready ? t("available") : t("nvrNotYet")}</strong>
          <small>{t("nvrLiveStreams")}: {data.status.active_live_streams}</small>
        </div>
        <div className="metric">
          <span>{t("nvrRecordingRuntime")}</span>
          <strong>{data.status.recording_ready ? t("available") : t("nvrNotYet")}</strong>
          <small>{t("nvrActiveRecordings")}: {data.status.active_recordings}</small>
        </div>
      </div>

      {formError && <div className="form-error">{formError}</div>}

      {runtimeMessage && <div className="storage-success">{runtimeMessage}</div>}

      {(!data.status.media_runtime_ready || !data.status.live_runtime_ready || !data.status.recording_ready) && (
        <div className="notice warning">
          <p>{t("nvrRuntimeMissing")}</p>
          {runtimeManage && (
            <div className="nvr-form-actions">
              <button
                type="button"
                className="button primary"
                disabled={Boolean(busy)}
                onClick={() => void installRuntime()}
              >
                {busy === "runtime:install" ? t("nvrRuntimeInstalling") : t("nvrRuntimeInstall")}
              </button>
            </div>
          )}
        </div>
      )}

      {storageManage && (
        <Panel title={t("nvrVideoStorage")}>
          {data.storage.length === 0 ? (
            <EmptyState>{t("nvrNoVideoStorage")}</EmptyState>
          ) : (
            <>
              <div className="nvr-storage-grid">
                <label>
                  {t("nvrStorageDevice")}
                  <select
                    value={selectedStorageDevice}
                    onChange={(event) => {
                      setStorageMessage("");
                      setStorageDevice(event.target.value);
                      const target = data.storage.find((item) => item.device === event.target.value);
                      setStorageReserve(target?.active ? target.reserve_percent : 5);
                    }}
                  >
                    {data.storage.map((target) => (
                      <option key={target.device} value={target.device} disabled={!target.ready}>
                        {target.label || target.device}
                        {target.mountpoint ? ` · ${target.mountpoint}` : ""}
                        {!target.ready ? ` · ${t("nvrStorageNotMounted")}` : ""}
                        {target.active ? ` · ${t("nvrStorageActive")}` : ""}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {t("nvrStorageReserve")}
                  <input
                    type="number"
                    min={1}
                    max={50}
                    value={selectedStorageReserve}
                    onChange={(event) => setStorageReserve(Number(event.target.value))}
                  />
                </label>
              </div>
              {storageMessage && <div className="storage-success">{storageMessage}</div>}
              {selectedStorage && (
                <div className="notice">
                  <p>
                    {selectedStorage.active && <><span className="status-badge status-success">{t("nvrStorageActive")}</span>{" · "}</>}
                    {t("nvrStorageTotal")}: {selectedStorage.size_bytes ? formatStorageBytes(selectedStorage.size_bytes) : "—"}
                    {" · "}{t("nvrStorageUsed")}: {selectedStorageUsed !== undefined ? formatStorageBytes(selectedStorageUsed) : "—"}
                    {" · "}{t("nvrStorageFree")}: {selectedStorage.free_known ? formatStorageBytes(selectedStorage.free_bytes) : "—"}
                    {" · "}{t("nvrStorageArchive")}: {formatStorageBytes(selectedStorage.archive_bytes)}
                    {" · "}{t("nvrStorageReserve")}: {selectedStorageReserve}%
                  </p>
                </div>
              )}
              <div className="nvr-form-actions">
                <button
                  type="button"
                  className="button primary"
                  disabled={Boolean(busy) || !selectedStorageDevice}
                  onClick={() => void saveStorage()}
                >
                  {busy === "storage" ? t("working") : selectedStorage?.active ? t("nvrStorageUpdate") : t("nvrStorageSave")}
                </button>
              </div>
            </>
          )}
        </Panel>
      )}

      <Panel title={t("nvrLive")}>
        {!data.status.live_runtime_ready ? (
          <div className="notice warning"><p>{t("nvrLiveUnavailable")}</p></div>
        ) : data.cameras.filter((camera) => camera.enabled && hasCameraLive(actor, camera.id)).length === 0 ? (
          <EmptyState>{t("nvrNoLiveCameras")}</EmptyState>
        ) : (
          <>
            <div className="nvr-toolbar nvr-live-toolbar">
              <button
                type="button"
                className="button secondary compact"
                onClick={() => openAllLive(data.cameras)}
              >
                {t("nvrOpenAllLive")}
              </button>
              {liveIDs.length > 0 && (
                <button type="button" className="button secondary compact" onClick={closeAllLive}>
                  {t("nvrCloseAllLive")}
                </button>
              )}
            </div>
            {liveIDs.length === 0 ? (
              <div className="notice"><p>{t("nvrLiveHint")}</p></div>
            ) : (
              <div className="nvr-live-grid">
                {data.cameras.filter((camera) => liveIDs.includes(camera.id)).map((camera) => (
                  <figure className="nvr-live-tile" key={camera.id}>
                    <div className="nvr-live-frame">
                      <img
                        src={api.nvrLiveURL(camera.id)}
                        alt={camera.name}
                        onLoad={() => setLiveErrors((items) => {
                          const next = {...items};
                          delete next[camera.id];
                          return next;
                        })}
                        onError={() => setLiveErrors((items) => ({...items, [camera.id]: true}))}
                      />
                      {liveErrors[camera.id] && (
                        <div className="nvr-live-error">{t("nvrLiveFailed")}</div>
                      )}
                    </div>
                    <figcaption>
                      <strong>{camera.name}</strong>
                      <button type="button" className="button secondary compact" onClick={() => toggleLive(camera)}>
                        {t("nvrHideLive")}
                      </button>
                    </figcaption>
                  </figure>
                ))}
              </div>
            )}
          </>
        )}
      </Panel>

      <Panel title={t("cameraList")}>
        {globalManage && !editorOpen && !onvifOpen && (
          <div className="nvr-toolbar">
            <button type="button" className="button primary" onClick={openCreate}>
              {t("nvrAddCamera")}
            </button>
            <button
              type="button"
              className="button secondary"
              disabled={Boolean(busy) || !data.status.onvif_ready}
              onClick={() => void discoverONVIF()}
            >
              {busy === "onvif:discover" ? t("working") : t("nvrDiscoverONVIF")}
            </button>
          </div>
        )}

        {data.cameras.length === 0 ? (
          <EmptyState>{t("nvrNoCameras")}</EmptyState>
        ) : (
          <div className="list">
            {data.cameras.map((camera) => {
              const canManage = hasCameraManage(actor, camera.id);
              const runtimeLabel =
                camera.runtime.state === "online"
                  ? t("nvrRuntimeOnline")
                  : camera.runtime.state === "offline"
                    ? t("nvrRuntimeOffline")
                    : camera.runtime.state === "connecting"
                      ? t("nvrRuntimeConnecting")
                      : t("nvrRuntimeDisabled");
              return (
                <div className="list-row nvr-camera-row" key={camera.id}>
                  <div>
                    <strong>{camera.name}</strong>
                    <span>
                      {camera.source_type.toUpperCase()} · {camera.transport.toUpperCase()} · {t("nvrRecording")}: {camera.recording_mode}
                      {" · "}{camera.has_credentials ? t("nvrCredentialsSaved") : t("nvrNoCredentials")}
                      {" · "}{runtimeLabel}
                      {camera.runtime.last_seen_at ? <>{" · "}{t("nvrLastSeen")}: {date(camera.runtime.last_seen_at)}</> : null}
                      {camera.runtime.reconnect_count > 0 ? <>{" · "}{t("nvrReconnects")}: {camera.runtime.reconnect_count}</> : null}
                      {camera.recording.active ? <>{" · "}{t("nvrRecordingActive")}</> : null}
                      {" · "}{t("modified")}: {date(camera.updated_at)}
                    </span>
                    {camera.runtime.last_error && (
                      <span className="muted">{camera.runtime.last_error}</span>
                    )}
                    {camera.recording.last_error && (
                      <span className="muted">{camera.recording.last_error}</span>
                    )}
                  </div>
                  <div className="nvr-camera-actions">
                    <Status value={camera.runtime.state} />
                    {camera.enabled && hasCameraLive(actor, camera.id) && (
                      <button
                        type="button"
                        className={liveIDs.includes(camera.id) ? "button primary compact" : "button secondary compact"}
                        disabled={!data.status.live_runtime_ready}
                        onClick={() => toggleLive(camera)}
                      >
                        {liveIDs.includes(camera.id) ? t("nvrHideLive") : t("nvrShowLive")}
                      </button>
                    )}
                    {canManage && (
                      <>
                        <button
                          type="button"
                          className="button secondary compact"
                          disabled={Boolean(busy)}
                          onClick={() => void testSavedCamera(camera)}
                        >
                          {busy === "test:" + camera.id ? t("working") : t("nvrTestConnection")}
                        </button>
                        <button
                          type="button"
                          className="button secondary compact"
                          disabled={Boolean(busy)}
                          onClick={() => void openEdit(camera)}
                        >
                          {busy === "load:" + camera.id ? t("working") : t("nvrEditCamera")}
                        </button>
                        <button
                          type="button"
                          className="button danger compact"
                          disabled={Boolean(busy)}
                          onClick={() => void deleteCamera(camera)}
                        >
                          {busy === "delete:" + camera.id ? t("working") : t("delete")}
                        </button>
                      </>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Panel>

      {onvifOpen && (
        <Panel title={t("nvrONVIFDiscovery")} className="wide">
          {onvifDevices.length === 0 ? (
            <div className="notice"><p>{t("nvrONVIFNoDevices")}</p></div>
          ) : !onvifDevice ? (
            <div className="list">
              {onvifDevices.map((device) => (
                <div className="list-row" key={device.id}>
                  <div>
                    <strong>{device.name}</strong>
                    <span>{device.ip} · {device.address}</span>
                  </div>
                  <button type="button" className="button primary compact" onClick={() => chooseONVIFDevice(device)}>
                    {t("select")}
                  </button>
                </div>
              ))}
            </div>
          ) : (
            <form className="nvr-camera-form" onSubmit={importONVIF}>
              <div className="notice">
                <strong>{onvifDevice.name}</strong>
                <span>{onvifDevice.ip}</span>
              </div>
              <label>
                {t("nvrCameraName")}
                <input required maxLength={128} value={onvifName} onChange={(event) => setONVIFName(event.target.value)} />
              </label>
              <label>
                {t("username")}
                <input autoComplete="off" value={onvifUsername} onChange={(event) => {
                  setONVIFUsername(event.target.value);
                  setONVIFProfiles([]);
                }} />
              </label>
              <label>
                {t("password")}
                <input type="password" autoComplete="new-password" value={onvifPassword} onChange={(event) => {
                  setONVIFPassword(event.target.value);
                  setONVIFProfiles([]);
                }} />
              </label>

              {onvifProfiles.length === 0 ? (
                <div className="nvr-form-actions">
                  <button type="button" className="button secondary" disabled={Boolean(busy)} onClick={() => setONVIFDevice(undefined)}>
                    {t("back")}
                  </button>
                  <button type="button" className="button primary" disabled={Boolean(busy)} onClick={() => void loadONVIFProfiles()}>
                    {busy === "onvif:profiles" ? t("working") : t("nvrLoadONVIFProfiles")}
                  </button>
                  <button type="button" className="button secondary" disabled={Boolean(busy)} onClick={resetONVIF}>
                    {t("cancel")}
                  </button>
                </div>
              ) : (
                <>
                  <label>
                    {t("nvrMainStream")}
                    <select value={onvifMainToken} onChange={(event) => {
                      const value = event.target.value;
                      setONVIFMainToken(value);
                      if (onvifSubToken === value) setONVIFSubToken("");
                    }}>
                      {onvifProfiles.map((profile) => (
                        <option key={profile.token} value={profile.token}>
                          {profile.name || profile.token} · {profile.width || 0}×{profile.height || 0} · {(profile.codec || "").toUpperCase()}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    {t("nvrSubstream")}
                    <select value={onvifSubToken} onChange={(event) => setONVIFSubToken(event.target.value)}>
                      <option value="">{t("nvrNoSubstream")}</option>
                      {onvifProfiles.filter((profile) => profile.token !== onvifMainToken).map((profile) => (
                        <option key={profile.token} value={profile.token}>
                          {profile.name || profile.token} · {profile.width || 0}×{profile.height || 0} · {(profile.codec || "").toUpperCase()}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    {t("nvrTransport")}
                    <select value={onvifTransport} onChange={(event) => setONVIFTransport(event.target.value as "tcp" | "udp")}>
                      <option value="tcp">TCP</option>
                      <option value="udp">UDP</option>
                    </select>
                  </label>
                  <label>
                    {t("nvrRecording")}
                    <select value={onvifRecordingMode} onChange={(event) => setONVIFRecordingMode(event.target.value as "off" | "continuous" | "motion")}>
                      <option value="off">{t("nvrRecordingOff")}</option>
                      <option value="continuous">{t("nvrRecordingContinuous")}</option>
                      <option value="motion">{t("nvrRecordingMotion")}</option>
                    </select>
                  </label>
                  <label className="nvr-inline-check">
                    <input type="checkbox" checked={onvifAudioEnabled} onChange={(event) => setONVIFAudioEnabled(event.target.checked)} />
                    <span>{t("nvrAudio")}</span>
                  </label>
                  <div className="nvr-form-actions">
                    <button type="button" className="button secondary" disabled={Boolean(busy)} onClick={() => {
                      setONVIFProfiles([]);
                      setONVIFMainToken("");
                      setONVIFSubToken("");
                    }}>
                      {t("back")}
                    </button>
                    <button type="button" className="button secondary" disabled={Boolean(busy)} onClick={resetONVIF}>
                      {t("cancel")}
                    </button>
                    <button type="submit" className="button primary" disabled={Boolean(busy) || !onvifName.trim() || !onvifMainToken}>
                      {busy === "onvif:import" ? t("working") : t("nvrImportONVIF")}
                    </button>
                  </div>
                </>
              )}
            </form>
          )}
          {!onvifDevice && (
            <div className="nvr-form-actions">
              <button type="button" className="button secondary" onClick={() => void discoverONVIF()}>
                {t("refresh")}
              </button>
              <button type="button" className="button secondary" onClick={resetONVIF}>
                {t("cancel")}
              </button>
            </div>
          )}
        </Panel>
      )}

      {editorOpen && (
        <Panel title={editingID ? t("nvrEditCamera") : t("nvrAddCamera")} className="wide">
          <form className="nvr-camera-form" onSubmit={save}>
            <label>
              {t("nvrCameraName")}
              <input
                required
                maxLength={128}
                value={editor.name}
                onChange={(event) => setEditor({...editor, name: event.target.value})}
              />
            </label>
            <label className="nvr-address-field">
              {t("nvrRTSPAddress")}
              <input
                required
                inputMode="url"
                placeholder="rtsp://192.168.1.100:554/stream1"
                value={editor.address}
                onChange={(event) => {
                  setEditor({...editor, address: event.target.value});
                  setProbe(undefined);
                  setTestedFingerprint("");
                }}
              />
              <small>{t("nvrCredentialsSeparateHint")}</small>
            </label>
            <label className="nvr-address-field">
              {t("nvrSubstreamAddress")}
              <input
                inputMode="url"
                placeholder="rtsp://192.168.1.100:554/stream2"
                value={editor.substreamAddress}
                disabled={editor.clearSubstream}
                onChange={(event) => {
                  setEditor({
                    ...editor,
                    substreamAddress: event.target.value,
                    clearSubstream: false,
                  });
                  setProbe(undefined);
                  setTestedFingerprint("");
                }}
              />
              <small>{t("nvrSubstreamHint")}</small>
            </label>
            <label>
              {t("username")}
              <input
                autoComplete="off"
                value={editor.username}
                placeholder={editingID ? t("nvrKeepSavedCredentials") : ""}
                onChange={(event) => {
                  setEditor({...editor, username: event.target.value, clearCredentials: false});
                  setProbe(undefined);
                  setTestedFingerprint("");
                }}
              />
            </label>
            <label>
              {t("password")}
              <input
                type="password"
                autoComplete="new-password"
                value={editor.password}
                placeholder={editingID ? t("nvrKeepSavedCredentials") : ""}
                onChange={(event) => {
                  setEditor({...editor, password: event.target.value, clearCredentials: false});
                  setProbe(undefined);
                  setTestedFingerprint("");
                }}
              />
            </label>
            <label>
              {t("nvrTransport")}
              <select
                value={editor.transport}
                onChange={(event) => {
                  setEditor({...editor, transport: event.target.value as "tcp" | "udp"});
                  setProbe(undefined);
                  setTestedFingerprint("");
                }}
              >
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </label>
            <label>
              {t("nvrRecording")}
              <select
                value={editor.recordingMode}
                onChange={(event) => setEditor({
                  ...editor,
                  recordingMode: event.target.value as "off" | "continuous" | "motion",
                })}
              >
                <option value="off">{t("nvrRecordingOff")}</option>
                <option value="continuous">{t("nvrRecordingContinuous")}</option>
                <option value="motion">{t("nvrRecordingMotion")}</option>
              </select>
            </label>
            <label className="nvr-inline-check">
              <input
                type="checkbox"
                checked={editor.audioEnabled}
                onChange={(event) => setEditor({...editor, audioEnabled: event.target.checked})}
              />
              <span>{t("nvrAudio")}</span>
            </label>
            {editingID && (
              <>
                <label className="nvr-inline-check">
                  <input
                    type="checkbox"
                    checked={editor.enabled}
                    onChange={(event) => setEditor({...editor, enabled: event.target.checked})}
                  />
                  <span>{t("nvrCameraEnabled")}</span>
                </label>
                <label className="nvr-inline-check">
                  <input
                    type="checkbox"
                    checked={editor.clearCredentials}
                    onChange={(event) => setEditor({
                      ...editor,
                      clearCredentials: event.target.checked,
                      username: event.target.checked ? "" : editor.username,
                      password: event.target.checked ? "" : editor.password,
                    })}
                  />
                  <span>{t("nvrClearCredentials")}</span>
                </label>
                <label className="nvr-inline-check">
                  <input
                    type="checkbox"
                    checked={editor.clearSubstream}
                    onChange={(event) => {
                      setEditor({
                        ...editor,
                        clearSubstream: event.target.checked,
                        substreamAddress: event.target.checked ? "" : editor.substreamAddress,
                      });
                      setProbe(undefined);
                      setTestedFingerprint("");
                    }}
                  />
                  <span>{t("nvrClearSubstream")}</span>
                </label>
              </>
            )}

            {probe && (
              <div className="notice nvr-probe-result">
                <strong>{t("nvrTestSuccess")}</strong>
                <span>{t("nvrMainStream")}: {probeText(probe)}</span>
                <span>{probe.has_audio ? t("nvrAudioDetected") : t("nvrNoAudioDetected")}</span>
                {probe.substream && (
                  <span>{t("nvrSubstream")}: {probeText(probe.substream)}</span>
                )}
              </div>
            )}

            <div className="nvr-form-actions">
              {!editingID && (
                <button
                  type="button"
                  className="button secondary"
                  disabled={Boolean(busy)}
                  onClick={() => void testConnection()}
                >
                  {busy === "test" ? t("working") : t("nvrTestConnection")}
                </button>
              )}
              <button
                type="button"
                className="button secondary"
                disabled={Boolean(busy)}
                onClick={resetEditor}
              >
                {t("cancel")}
              </button>
              <button
                type="submit"
                className="button primary"
                disabled={Boolean(busy) || (!editingID && !createProbeValid)}
              >
                {busy === "save" ? t("working") : editingID ? t("save") : t("nvrAddCamera")}
              </button>
            </div>
          </form>
        </Panel>
      )}

      <Panel title={t("nvrFoundation")}>
        <div className="notice">
          <p>{t("nvrFoundationNotice")}</p>
        </div>
      </Panel>
    </div>
  );
}
