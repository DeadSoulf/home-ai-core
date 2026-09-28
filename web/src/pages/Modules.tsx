import { useCallback } from "react";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { EmptyState, ErrorState, LoadingState, Panel } from "../components/Panel";
import { PageHeading, Status } from "./Dashboard";

export function ModulesPage({revision}: {revision: number}) {
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
      <PageHeading title="Modules" subtitle="Registered platform capabilities. Installation arrives in Phase 9." />

      <Panel title="Host capabilities">
        <div className="tag-list">
          {value.capabilities.map((capability) => <span className="tag" key={capability}>{capability}</span>)}
        </div>
      </Panel>

      <div className="card-grid">
        {value.modules.map((module) => (
          <article className="module-card" key={module.manifest.id}>
            <div className="module-title">
              <div>
                <h2>{module.manifest.name}</h2>
                <span className="mono">{module.manifest.id} · {module.manifest.version}</span>
              </div>
              <Status value={module.status} />
            </div>
            <p>{module.manifest.description || "No description."}</p>
            <div className="tag-list">
              {(module.manifest.capabilities?.provides || []).map((capability) => (
                <span className="tag" key={capability}>{capability}</span>
              ))}
            </div>
          </article>
        ))}
        {value.modules.length === 0 && (
          <EmptyState>No modules are registered yet. This is expected before the Module Store phase.</EmptyState>
        )}
      </div>
    </div>
  );
}
