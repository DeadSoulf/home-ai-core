import { useCallback, useState } from "react";
import type { RegisteredModule } from "../api/types";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { EmptyState, ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading, Status } from "./Dashboard";

export function ModulesPage({revision, canManage}: {revision: number; canManage: boolean}) {
  const {t} = useI18n();
  const [manifestText, setManifestText] = useState("");
  const [catalogToken, setCatalogToken] = useState("");
  const [actionError, setActionError] = useState("");
  const [queuedJob, setQueuedJob] = useState("");
  const [busy, setBusy] = useState(false);
  const [catalogBusy, setCatalogBusy] = useState(false);

  const load = useCallback(async () => {
    const [modules, capabilities, access] = await Promise.all([
      api.modules(),
      api.capabilities(),
      api.moduleCatalogAccess(),
    ]);
    let catalog;
    let catalogError = "";
    if (access.token_configured) {
      try {
        catalog = await api.moduleCatalog();
      } catch (reason: unknown) {
        catalogError = reason instanceof Error ? reason.message : t("requestFailed");
      }
    }
    return {modules, capabilities, access, catalog, catalogError};
  }, [t]);
  const {data, loading, error, reload} = useResource(load, revision);

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

  const saveCatalogToken = async () => {
    setCatalogBusy(true);
    setActionError("");
    try {
      await api.setModuleCatalogToken(catalogToken);
      setCatalogToken("");
      if (catalogToken.trim()) {
        await api.moduleCatalog(true);
      }
      reload();
    } catch (reason: unknown) {
      setActionError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setCatalogBusy(false);
    }
  };

  const refreshCatalog = async () => {
    setCatalogBusy(true);
    setActionError("");
    try {
      await api.moduleCatalog(true);
      reload();
    } catch (reason: unknown) {
      setActionError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setCatalogBusy(false);
    }
  };

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;
  const value = data!;
  const dockerReady = value.capabilities.includes("host.docker");
  const catalogModules = value.catalog?.modules || [];
  const availableModules = catalogModules.filter((module) => !module.installed || module.update_available);
  const catalogByID = new Map(catalogModules.map((module) => [module.manifest.id, module]));

  return (
    <div className="page">
      <PageHeading title={t("modules")} subtitle={t("modulesSubtitleSimple")} />
      {error && <ErrorState message={error} />}
      {queuedJob && <div className="notice">{t("moduleJobQueued")}: <span className="mono">{queuedJob}</span></div>}
      {actionError && <div className="form-error">{actionError}</div>}

      <Panel title={t("dockerRuntime")}>
        <div className="details">
          <div>{dockerReady ? t("dockerRuntimeReady") : t("dockerRuntimePreparing")}</div>
          <div className="mono">host.docker: {dockerReady ? "yes" : "no"}</div>
        </div>
      </Panel>

      <Panel title={t("moduleCatalogAvailable")}>
        <p>{t("moduleCatalogHint")}</p>
        <div className="details">
          <div>{t("moduleCatalogRepository")}</div>
          <div className="mono">{value.access.repository}</div>
        </div>

        {!value.access.token_configured && canManage && (
          <div>
            <div className="notice">{t("moduleCatalogTokenRequired")}</div>
            <input
              className="text-input mono"
              type="password"
              value={catalogToken}
              onChange={(event) => setCatalogToken(event.target.value)}
              placeholder={t("moduleCatalogTokenPlaceholder")}
              autoComplete="off"
              disabled={catalogBusy}
            />
            <div className="button-row">
              <button
                type="button"
                className="button primary"
                disabled={catalogBusy || !catalogToken.trim()}
                onClick={() => void saveCatalogToken()}
              >
                {catalogBusy ? t("working") : t("moduleCatalogSaveToken")}
              </button>
            </div>
          </div>
        )}

        {value.access.token_configured && (
          <div className="button-row">
            <button
              type="button"
              className="button secondary"
              disabled={catalogBusy}
              onClick={() => void refreshCatalog()}
            >
              {catalogBusy ? t("working") : t("moduleCatalogRefresh")}
            </button>
          </div>
        )}

        {value.catalogError && <ErrorState message={value.catalogError} />}
        {value.catalog?.warning && <div className="notice">{value.catalog.warning}</div>}

        {availableModules.length > 0 ? (
          <div className="card-grid">
            {availableModules.map((module) => (
              <article className="module-card" key={module.manifest.id}>
                <div className="module-title">
                  <div>
                    <h2>{module.manifest.name}</h2>
                  </div>
                  <Status value={module.installed ? (module.installed_status || "registered") : "available"} />
                </div>
                <p>{module.manifest.description || t("noDescription")}</p>
                <dl className="details">
                  <dt>ID</dt><dd className="mono">{module.manifest.id}</dd>
                  <dt>{t("moduleLatestVersion")}</dt><dd>{module.manifest.version}</dd>
                  {module.installed_version && (
                    <>
                      <dt>{t("moduleInstalledVersion")}</dt><dd>{module.installed_version}</dd>
                    </>
                  )}
                  <dt>{t("coreVersion")}</dt><dd>{module.manifest.core}</dd>
                </dl>
                {!module.compatible && (
                  <div className="form-error">{t("moduleIncompatible")}: {module.compatibility_error}</div>
                )}
                {canManage && (
                  <div className="button-row">
                    {!module.installed ? (
                      <button
                        type="button"
                        className="button primary"
                        disabled={busy || !module.compatible}
                        onClick={() => void run(() => api.installCatalogModule(module.manifest.id))}
                      >
                        {t("moduleInstall")}
                      </button>
                    ) : module.update_available ? (
                      <button
                        type="button"
                        className="button primary"
                        disabled={busy || !module.compatible}
                        onClick={() => void run(() => api.updateCatalogModule(module.manifest.id))}
                      >
                        {t("moduleUpdate")}
                      </button>
                    ) : null}
                  </div>
                )}
              </article>
            ))}
          </div>
        ) : value.access.token_configured && !value.catalogError ? (
          <EmptyState>{t("moduleCatalogNoAvailable")}</EmptyState>
        ) : null}
      </Panel>

      <Panel title={t("moduleInstalled")}>
        <div className="card-grid">
          {value.modules.map((module) => {
            const catalogModule = catalogByID.get(module.manifest.id);
            return (
              <article className="module-card" key={module.manifest.id}>
                <div className="module-title">
                  <div>
                    <h2>{module.manifest.name}</h2>
                  </div>
                  <Status value={module.status} />
                </div>
                <p>{module.manifest.description || t("noDescription")}</p>
                {module.error && <div className="form-error">{module.error}</div>}
                {catalogModule?.update_available && (
                  <div className="notice">
                    {t("moduleUpdateAvailable")}: {catalogModule.manifest.version}
                  </div>
                )}

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
                    {catalogModule?.update_available && (
                      <button
                        type="button"
                        className="button primary"
                        disabled={busy || !catalogModule.compatible}
                        onClick={() => void run(() => api.updateCatalogModule(module.manifest.id))}
                      >
                        {t("moduleUpdate")}
                      </button>
                    )}
                    <button type="button" className="button danger" disabled={busy}
                      onClick={() => void run(() => api.removeModule(module.manifest.id))}>
                      {t("moduleRemove")}
                    </button>
                    <button
                      type="button"
                      className="button danger"
                      disabled={busy}
                      onClick={() => {
                        if (window.confirm(t("moduleRemoveDataConfirm"))) {
                          void run(() => api.removeModule(module.manifest.id, true));
                        }
                      }}
                    >
                      {t("moduleRemoveData")}
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
            );
          })}
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
        {canManage && value.access.token_configured && (
          <Panel title={t("moduleCatalogAccess")}>
            <p>{t("moduleCatalogReplaceTokenHint")}</p>
            <input
              className="text-input mono"
              type="password"
              value={catalogToken}
              onChange={(event) => setCatalogToken(event.target.value)}
              placeholder={t("moduleCatalogTokenPlaceholder")}
              autoComplete="off"
              disabled={catalogBusy}
            />
            <div className="button-row">
              <button
                type="button"
                className="button secondary"
                disabled={catalogBusy || !catalogToken.trim()}
                onClick={() => void saveCatalogToken()}
              >
                {catalogBusy ? t("working") : t("moduleCatalogReplaceToken")}
              </button>
            </div>
          </Panel>
        )}
      </details>

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
