import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { EmptyState, ErrorState, LoadingState, Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading, Status } from "./Dashboard";

export function ModulesPage({revision}: {revision: number}) {
  const {t} = useI18n();
  const load = useCallback(async () => {
    const [modules, capabilities] = await Promise.all([api.modules(), api.capabilities()]);
    return {modules, capabilities};
  }, []);
  const {data, loading, error} = useResource(load, revision);

  if (loading && !data) return <LoadingState />;
  if (error && !data) return <ErrorState message={error} />;
  const value = data!;

  return (
    <div className="page">
      <PageHeading title={t("modules")} subtitle={t("modulesSubtitleSimple")} />
      {error && <ErrorState message={error} />}

      <div className="card-grid">
        {value.modules.map((module) => (
          <article className="module-card" key={module.manifest.id}>
            <div className="module-title">
              <div><h2>{module.manifest.name}</h2></div>
              <Status value={module.status} />
            </div>
            <p>{module.manifest.description || t("noDescription")}</p>
            {module.error && <div className="form-error">{module.error}</div>}
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
        {value.modules.length === 0 && <EmptyState>{t("noModulesSimple")}</EmptyState>}
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
