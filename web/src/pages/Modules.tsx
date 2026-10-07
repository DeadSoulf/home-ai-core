import { useCallback, useState } from "react";
import type { RegisteredModule } from "../api/types";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { EmptyState, ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading, Status } from "./Dashboard";

type CatalogState = {
  id: string;
  generated_at?: string;
  modules: RegisteredModule["manifest"][];
};

export function ModulesPage({revision, canManage}: {revision: number; canManage: boolean}) {
  const {t} = useI18n();
  const [manifestText, setManifestText] = useState("");
  const [actionError, setActionError] = useState("");
  const [queuedJob, setQueuedJob] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    const [modules, capabilities] = await Promise.all([api.modules(), api.capabilities()]);
    let catalog: CatalogState | undefined;
    let catalogError = "";
    try {
      catalog = await api.moduleCatalog();
    } catch (reason: unknown) {
      catalogError = reason instanceof Error ? reason.message : t("moduleCatalogUnavailable");
    }
    return {modules, capabilities, catalog, catalogError};
  }, [t]);
  const {data, loading, error} = useResource(load, revision);

  const run = async (action: () => Promise<{id: string}>) => {
    setBusy(true);
    setActionError("");
    try {
      const job = await action();
      setQueuedJob(job.id);
    } catch (reason: unknown) {
      setActionError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy(false);
    }
  };

  const installManifest = async () => {
    let manifest: RegisteredModule["manifest"];
    try {
      manifest = JSON.parse(manifestText) as RegisteredModule["manifest"];
    } catch {
      setActionError(t("moduleManifestInvalidJSON"));
      return;
    }
    await run(() => api.installModule(manifest));
  };

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;
  const value = data!;
  const dockerReady = value.capabilities.includes("host.docker");
  const installedByID = new Map(value.modules.map((module) => [module.manifest.id, module]));

  return (
    <div className="page">
      <PageHeading title={t("modules")} subtitle={t("modulesSubtitleSimple")} />
      {error && <ErrorState message={error} />}

      <Panel title={t("dockerRuntime")}>
        <div className="details">
          <div>{dockerReady ? t("dockerRuntimeReady") : t("dockerRuntimePreparing")}</div>
          <div className="mono">host.docker: {dockerReady ? "yes" : "no"}</div>
        </div>
      </Panel>

      <Panel title={t("moduleCatalogAvailable")}>
        {value.catalogError && <div className="form-error">{value.catalogError}</div>}
        {value.catalog && (
          <>
            <div className="details">
              <div>{t("moduleCatalogOfficial")}</div>
              <div className="mono">{value.catalog.id}</div>
            </div>
            <div className="card-grid">
              {value.catalog.modules.map((manifest) => {
                const installed = installedByID.get(manifest.id);
                const updateAvailable = Boolean(installed && installed.manifest.version !== manifest.version);
                return (
                  <article className="module-card" key={manifest.id}>
                    <div className="module-title">
                      <div><h2>{manifest.name}</h2></div>
                      <span className="tag">
                        {installed ? (updateAvailable ? t("moduleUpdateAvailable") : t("moduleInstalled")) : t("available")}
                      </span>
                    </div>
                    <p>{manifest.description || t("noDescription")}</p>
                    <dl className="details">
                      <dt>{t("availableVersion")}</dt><dd>{manifest.version}</dd>
                      {installed && <><dt>{t("currentVersion")}</dt><dd>{installed.manifest.version}</dd></>}
                      <dt>{t("coreVersion")}</dt><dd>{manifest.core}</dd>
                    </dl>
                    {canManage && (
                      <div className="button-row">
                        <button
                          type="button"
                          className="button primary"
                          disabled={busy || !dockerReady || Boolean(installed && !updateAvailable)}
                          onClick={() => void run(() => api.installCatalogModule(manifest.id))}
                        >
                          {busy ? t("working") : updateAvailable ? t("moduleUpdate") : installed ? t("moduleInstalled") : t("moduleInstall")}
                        </button>
                      </div>
                    )}
                  </article>
                );
              })}
              {value.catalog.modules.length === 0 && <EmptyState>{t("moduleCatalogEmpty")}</EmptyState>}
            </div>
          </>
        )}
      </Panel>

      <Panel title={t("moduleInstalledList")}>
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

              {canManage && (
                <div className="button-row">
                  {module.status === "disabled" ? (
                    <button type="button" className="button primary" disabled={busy}
                      onClick={() => void run(() => api.controlModule(module.manifest.id, "enable"))}>
                      {t("moduleStart")}
                    </button>
                  ) : (
                    <button type="button" className="button secondary" disabled={busy}
                      onClick={() => void run(() => api.controlModule(module.manifest.id, "disable"))}>
                      {t("moduleStop")}
                    </button>
                  )}
                  <button type="button" className="button secondary" disabled={busy}
                    onClick={() => void run(() => api.controlModule(module.manifest.id, "restart"))}>
                    {t("moduleRestart")}
                  </button>
                  <button type="button" className="button danger" disabled={busy}
                    onClick={() => void run(() => api.removeModule(module.manifest.id))}>
                    {t("moduleRemove")}
                  </button>
                </div>
              )}

              <details className="technical-details">
                <summary>{t("technicalDetails")}</summary>
                <dl className="details">
                  <dt>ID</dt><dd className="mono">{module.manifest.id}</dd>
                  <dt>{t("version")}</dt><dd>{module.manifest.version}</dd>
                  <dt>{t("coreVersion")}</dt><dd>{module.manifest.core}</dd>
                  <dt>{t("moduleImage")}</dt><dd className="mono">{module.manifest.runtime?.docker?.image || "—"}</dd>
                  <dt>{t("hostCapabilities")}</dt><dd className="mono">{(module.manifest.capabilities?.provides || []).join(", ") || "—"}</dd>
                </dl>
              </details>
            </article>
          ))}
          {value.modules.length === 0 && (
            <EmptyState>{t("noModulesSimple")}</EmptyState>
          )}
        </div>
      </Panel>

      <details className="technical-details">
        <summary>{t("technicalDetails")}</summary>
        <Panel title={t("hostCapabilities")}>
          <div className="tag-list">
            {value.capabilities.map((capability) => <span className="tag" key={capability}>{capability}</span>)}
          </div>
        </Panel>
      </details>

      {queuedJob && <div className="notice">{t("moduleJobQueued")}: <span className="mono">{queuedJob}</span></div>}
      {actionError && <div className="form-error">{actionError}</div>}

      {canManage && (
        <Panel title={t("moduleInstallManifest")}>
          <p>{t("moduleInstallManifestHint")}</p>
          <textarea
            className="text-input mono"
            rows={12}
            value={manifestText}
            onChange={(event) => setManifestText(event.target.value)}
            placeholder={'{"schema_version":2,...}'}
            disabled={busy}
          />
          <div className="button-row">
            <button
              type="button"
              className="button primary"
              disabled={busy || !manifestText.trim() || !dockerReady}
              onClick={() => void installManifest()}
            >
              {busy ? t("working") : t("moduleInstall")}
            </button>
          </div>
        </Panel>
      )}
    </div>
  );
}
