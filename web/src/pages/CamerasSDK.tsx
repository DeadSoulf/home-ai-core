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

  const [address, setAddress] = useState("");
  const [port, setPort] = useState("8000");
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [testError, setTestError] = useState("");
  const [testMessage, setTestMessage] = useState("");

  const testLogin = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage || busy) return;
    setBusy(true);
    setTestError("");
    setTestMessage("");
    try {
      const parsedPort = Number(port);
      const result = await api.testCameraSDKLogin({
        address: address.trim(),
        port: Number.isFinite(parsedPort) ? parsedPort : undefined,
        username: username.trim(),
        password,
      });
      if (result.ok) {
        setTestMessage(t("camerasSDKTestSuccess"));
      }
    } catch (reason) {
      setTestError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy(false);
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
          <div className="notice">
            {t("camerasSDKInstallHint")}
            <br />
            <code>/opt/home-ai/hikvision/lib/libhcnetsdk.so</code>
          </div>
        )}
      </Panel>

      <Panel title={t("camerasSDKConnectionTest")}>
        <p className="muted">{t("camerasSDKConnectionHint")}</p>
        {testError && <ErrorState message={testError} />}
        {testMessage && <div className="notice success">{testMessage}</div>}
        <form className="network-profile-form" onSubmit={testLogin}>
          <label>
            {t("camerasSDKAddress")}
            <input
              value={address}
              onChange={(event) => setAddress(event.target.value)}
              placeholder="192.168.1.64"
              autoComplete="off"
              required
            />
          </label>
          <label>
            {t("camerasSDKPort")}
            <input
              type="number"
              min="1"
              max="65535"
              value={port}
              onChange={(event) => setPort(event.target.value)}
              required
            />
          </label>
          <label>
            {t("username")}
            <input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoComplete="username"
              required
            />
          </label>
          <label>
            {t("password")}
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
            />
          </label>
          <button
            type="submit"
            className="button primary"
            disabled={!canManage || busy || !sdkReady}
          >
            {busy ? t("working") : t("camerasSDKTest")}
          </button>
        </form>
        {!canManage && <p className="muted">{t("camerasSDKManageRequired")}</p>}
      </Panel>
    </div>
  );
}
