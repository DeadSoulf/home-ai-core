import {useCallback} from "react";
import {api} from "../api/client";
import {EmptyState, ErrorState, LoadingState, Panel} from "../components/Panel";
import {useResource} from "../hooks/useResource";
import {useI18n} from "../i18n";
import {PageHeading, Status} from "./Dashboard";

export function CamerasPage({revision}: {revision: number}) {
  const {t, date} = useI18n();
  const load = useCallback(async () => {
    const [status, cameras] = await Promise.all([api.nvrStatus(), api.nvrCameras()]);
    return {status, cameras};
  }, []);
  const resource = useResource(load, revision);

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  const data = resource.data!;
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
          <span>{t("nvrMediaRuntime")}</span>
          <strong>{data.status.media_runtime_ready ? t("available") : t("nvrNotYet")}</strong>
          <small>{t("nvrMediaRuntimeHint")}</small>
        </div>
        <div className="metric">
          <span>{t("nvrCredentialStore")}</span>
          <strong>{data.status.secret_store_ready ? t("available") : t("nvrNotYet")}</strong>
          <small>{t("nvrCredentialStoreHint")}</small>
        </div>
      </div>

      <Panel title={t("cameraList")}>
        {data.cameras.length === 0 ? (
          <EmptyState>{t("nvrNoCameras")}</EmptyState>
        ) : (
          <div className="list">
            {data.cameras.map((camera) => (
              <div className="list-row" key={camera.id}>
                <div>
                  <strong>{camera.name}</strong>
                  <span>
                    {camera.source_type.toUpperCase()} · {camera.transport.toUpperCase()} · {t("nvrRecording")}: {camera.recording_mode}
                    {" · "}{t("modified")}: {date(camera.updated_at)}
                  </span>
                </div>
                <Status value={camera.enabled ? "enabled" : "disabled"} />
              </div>
            ))}
          </div>
        )}
      </Panel>

      <Panel title={t("nvrFoundation")}>
        <div className="notice">
          <p>{t("nvrFoundationNotice")}</p>
        </div>
      </Panel>
    </div>
  );
}
