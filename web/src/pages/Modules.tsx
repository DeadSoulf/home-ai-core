import { useCallback, useState } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { EmptyState, ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading, Status } from "./Dashboard";

export function ModulesPage({revision, canManage}: {revision: number; canManage: boolean}) {
  const {t} = useI18n();
  const load = useCallback(async () => {
    const [modules, capabilities] = await Promise.all([api.modules(), api.capabilities()]);
    return {modules, capabilities};
  }, []);
  const {data, loading, error, reload} = useResource(load, revision);
  const [controlBusy, setControlBusy] = useState("");
  const [controlError, setControlError] = useState("");
  const [controlMessage, setControlMessage] = useState("");

  const controlModule = async (id: string, operation: "enable" | "disable" | "restart") => {
    if (controlBusy) return;
    if (operation === "disable" && !window.confirm(t("moduleDisableConfirm"))) return;
    const busyKey = id + ":" + operation;
    setControlBusy(busyKey);
    setControlError("");
    setControlMessage("");
    try {
      await api.controlModule(id, operation);
      setControlMessage(
        operation === "enable"
          ? t("moduleEnabled")
          : operation === "disable"
            ? t("moduleDisabled")
            : t("moduleRestarted"),
      );
      reload();
      window.dispatchEvent(new CustomEvent("home-ai-core:modules-changed"));
    } catch (reason) {
      setControlError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setControlBusy("");
    }
  };

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;
  const value = data!;

  return (
    <div className="page">
      <PageHeading title={t("modules")} subtitle={t("modulesSubtitleSimple")} />
      {error && <ErrorState message={error} />}
      {controlError && <ErrorState message={controlError} />}
      {controlMessage && <div className="notice success">{controlMessage}</div>}

      <div className="card-grid">
        {value.modules.map((module) => (
          <article className="module-card" key={module.manifest.id}>
            <div className="module-title">
              <div>
                <h2>{module.manifest.name}</h2>
              </div>
              <Status value={module.status} />
            </div>
            <p>{module.manifest.description || t("noDescription")}</p>
            {module.error && <div className="form-error">{module.error}</div>}
            {(module.manifest.id === "ai.agent" || module.manifest.id === "ai.cloud") && canManage && (
              <div className="module-actions">
                {module.status === "disabled" || module.status === "error" || module.status === "registered" ? (
                  <button
                    type="button"
                    className="button primary"
                    disabled={Boolean(controlBusy)}
                    onClick={() => void controlModule(module.manifest.id, "enable")}
                  >
                    {controlBusy === module.manifest.id + ":enable" ? t("working") : t("moduleEnable")}
                  </button>
                ) : (
                  <button
                    type="button"
                    className="button secondary"
                    disabled={Boolean(controlBusy)}
                    onClick={() => void controlModule(module.manifest.id, "disable")}
                  >
                    {controlBusy === module.manifest.id + ":disable" ? t("working") : t("moduleDisable")}
                  </button>
                )}
                <button
                  type="button"
                  className="button secondary"
                  disabled={Boolean(controlBusy) || module.status !== "enabled"}
                  onClick={() => void controlModule(module.manifest.id, "restart")}
                >
                  {controlBusy === module.manifest.id + ":restart" ? t("working") : t("moduleRestart")}
                </button>
              </div>
            )}
            {module.manifest.id === "ai.agent" && (
              <p className="muted module-control-hint">
                {module.status === "enabled" ? t("moduleAIRunningHint") : t("moduleAIStoppedHint")}
              </p>
            )}
            {module.manifest.id === "ai.cloud" && (
              <p className="muted module-control-hint">
                {module.status === "enabled" ? t("moduleCloudRunningHint") : t("moduleCloudStoppedHint")}
              </p>
            )}
            <details className="technical-details">
              <summary>{t("technicalDetails")}</summary>
              <dl className="details">
                <dt>ID</dt><dd className="mono">{module.manifest.id}</dd>
                <dt>{t("version")}</dt><dd>{module.manifest.version}</dd>
                <dt>{t("coreVersion")}</dt><dd>{module.manifest.core}</dd>
                <dt>{t("hostCapabilities")}</dt><dd className="mono">{(module.manifest.capabilities?.provides || []).join(", ") || "—"}</dd>
              </dl>
            </details>
          </article>
        ))}
        {value.modules.length === 0 && (
          <EmptyState>{t("noModulesSimple")}</EmptyState>
        )}
      </div>
      <details className="technical-details">
        <summary>{t("technicalDetails")}</summary>
        <Panel title={t("hostCapabilities")}>
          <div className="tag-list">
            {value.capabilities.map((capability) => <span className="tag" key={capability}>{capability}</span>)}
          </div>
        </Panel>
      </details>
    </div>
  );
}
