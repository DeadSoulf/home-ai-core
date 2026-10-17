import { useCallback, useEffect, useRef, useState } from "react";
import type { Job, RegisteredModule } from "../api/types";
import { api } from "../api/client";
import { useResource } from "../hooks/useResource";
import { EmptyState, ErrorState, LoadingState, Panel } from "../components/Panel";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { ErrorNotice } from "../components/ErrorNotice";
import { JobProgress } from "../components/JobProgress";
import type { ModuleSection } from "../navigation";
import { useI18n } from "../i18n";
import { useUI } from "../ui/useUI";
import { activeJob, catalogState, jobTitle, runtimeMessage } from "../ui/moduleState";
import { PageHeading, Status } from "./Dashboard";

export function ModulesPage({revision, section = "installed", canManage, canReadJobs = false, onSectionChange = () => {}, onNavigate = () => {}}: {
  revision: number; section?: ModuleSection; canManage: boolean; canReadJobs?: boolean;
  onSectionChange?: (section: ModuleSection) => void; onNavigate?: (path: string) => void;
}) {
  const {t, date} = useI18n(); const u = useUI();
  const [manifestText, setManifestText] = useState("");
  const [manifestError, setManifestError] = useState(false);
  const [actionError, setActionError] = useState<unknown>();
  const [sending, setSending] = useState(false), [uncertain, setUncertain] = useState(false);
  const [accepted, setAccepted] = useState<Job>();
  const [confirmRemove, setConfirmRemove] = useState<RegisteredModule>();
  const mutation = useRef(false);
  // Catalog outages never hide the installed registry or stop/remove controls.
  const load = useCallback(() => Promise.all([api.modules(), api.capabilities()]).then(([modules, capabilities]) => ({modules, capabilities})), []);
  const core = useResource(load, revision);
  const loadCatalog = useCallback(() => api.moduleCatalog(), []);
  const catalog = useResource(loadCatalog);
  const loadRuntime = useCallback((signal: AbortSignal) => api.moduleRuntime(signal), []);
  const runtime = useResource(loadRuntime, 0, 15000);
  const loadOperations = useCallback((signal: AbortSignal) => canManage ? api.moduleOperations(signal) : Promise.resolve<Job[]>([]), [canManage]);
  const operations = useResource(loadOperations, revision, canManage ? 2000 : 0);
  const jobID = accepted?.id;
  const loadJob = useCallback((signal: AbortSignal) => jobID ? api.moduleOperation(jobID, signal) : Promise.resolve<Job | undefined>(undefined), [jobID]);
  const tracked = useResource(loadJob, revision, jobID ? 2000 : 0);
  const lastJob = tracked.data?.id === jobID ? tracked.data : accepted;
  const active = (operations.data || []).filter(activeJob);
  const monitoringFailed = Boolean(operations.error || (jobID && tracked.error));
  const busy = sending || uncertain || monitoringFailed || !operations.data || active.length > 0 || Boolean(lastJob && activeJob(lastJob));
  const fingerprint = active.map((job) => job.id + job.status).sort().join(";") + (lastJob?.status || "");
  useEffect(() => {
    core.reload(); runtime.reload();
    window.dispatchEvent(new CustomEvent("home-ai-core:modules-changed"));
  }, [fingerprint, core.reload, runtime.reload]);

  async function verifyStatus() {
    if (mutation.current) return;
    mutation.current = true; setSending(true);
    try {
      const jobs = await api.moduleOperations();
      if (jobID) {
        try { setAccepted(await api.moduleOperation(jobID)); }
        catch (error) { if ((error as {status?: number}).status !== 404) throw error; setAccepted(undefined); }
      }
      else if (jobs[0]) setAccepted(jobs[0]);
      setUncertain(false);
      operations.reload(); tracked.reload(); core.reload(); runtime.reload();
    } catch (error) { setActionError(error); }
    finally { mutation.current = false; setSending(false); }
  }

  async function run(action: () => Promise<Job>) {
    if (!canManage || busy || mutation.current || core.error) return;
    mutation.current = true; setSending(true); setActionError(undefined);
    try {
      const job = await action();
      setAccepted(job);
      operations.reload();
      onSectionChange("operations");
    }
    catch (error) {
      setActionError(error); setUncertain(true); // a lost response is not proof that nothing was queued
      operations.reload(); core.reload();
    } finally { mutation.current = false; setSending(false); }
  }

  function installManifest() {
    setManifestError(false);
    try {
      if (new TextEncoder().encode(manifestText).length > 256 * 1024) throw new Error("too large");
      const manifest = JSON.parse(manifestText) as RegisteredModule["manifest"];
      if (!manifest || typeof manifest !== "object" || Array.isArray(manifest) || typeof manifest.id !== "string" || !manifest.id.trim()) throw new Error("invalid object");
      void run(() => api.installModule(manifest));
    } catch { setManifestError(true); document.getElementById("module-manifest")?.focus(); }
  }

  if (core.loading && !core.data) return <LoadingState />;
  if (!core.data) return <ErrorState message={core.error} onRetry={core.reload} />;
  const value = core.data;
  const installed = new Map(value.modules.map((item) => [item.manifest.id, item]));
  const displayedJobs = [...active];
  if (lastJob && !displayedJobs.some((job) => job.id === lastJob.id)) displayedJobs.push(lastJob);
  return <div className="page modules-page">
    <PageHeading title={t("modules")} subtitle={u(section === "catalog" ? "moduleCatalogSubtitle" : section === "operations" ? "moduleOperationsSubtitle" : section === "settings" ? "moduleSettingsSubtitle" : "moduleInstalledSubtitle")} />
    <nav className="ux-section-jumps module-section-tabs" aria-label={t("modules")}>
      <button type="button" className={"button " + (section === "installed" ? "primary" : "secondary")}
        aria-current={section === "installed" ? "page" : undefined} onClick={() => onSectionChange("installed")}>{u("uxInstalledJump")}</button>
      <button type="button" className={"button " + (section === "catalog" ? "primary" : "secondary")}
        aria-current={section === "catalog" ? "page" : undefined} onClick={() => onSectionChange("catalog")}>{u("uxCatalogJump")}</button>
      <button type="button" className={"button " + (section === "operations" ? "primary" : "secondary")}
        aria-current={section === "operations" ? "page" : undefined} onClick={() => onSectionChange("operations")}>{u("uxOperationsJump")}</button>
      <button type="button" className={"button " + (section === "settings" ? "primary" : "secondary")}
        aria-current={section === "settings" ? "page" : undefined} onClick={() => onSectionChange("settings")}>{u("uxModuleSettingsJump")}</button>
    </nav>
    {core.error && <ErrorState message={core.error} onRetry={core.reload} />}
    {section === "settings" && <Panel title={t("dockerRuntime")} action={<button type="button" className="button secondary" disabled={runtime.loading} onClick={runtime.reload}>{u("checkRuntime")}</button>}>
      <div className="runtime-summary">
        <p role="status">{u(runtimeMessage(runtime.data))}</p>
        {runtime.loading && !runtime.data && <LoadingState />}
        {runtime.error && <ErrorState message={runtime.error} />}
        {runtime.data?.checked_at && <p className="muted small">{u("lastUpdated")}: {date(runtime.data.checked_at)}</p>}
        {(!runtime.data?.engine_ready || !runtime.data?.network_ready) && <p>{u("runtimeInstallHint")}</p>}
        {runtime.data?.error && <details className="technical-details"><summary>{u("details")}</summary><p>{runtime.data.error}</p></details>}
      </div>
    </Panel>}

    {section === "operations" && canManage && !operations.data && !operations.error && <p role="status">{u("queueChecking")}</p>}
    {monitoringFailed && <ErrorState message={u("operationMonitorFailed")} onRetry={() => void verifyStatus()} />}
    {uncertain && <div className="notice" role="status"><p>{u("operationUnknown")}</p><button type="button" className="button secondary" disabled={sending} onClick={() => void verifyStatus()}>{u("refresh")}</button></div>}
    {actionError !== undefined && <ErrorNotice error={actionError} />}
    {section === "operations" && (displayedJobs.length > 0 ? <Panel title={u("activeOperation")}>
      <div className="module-operations">
        {displayedJobs.map((job) => <article key={job.id} className="operation-card" data-job-id={job.id}>
          <h3>{u(jobTitle(job))}: {installed.get(job.input?.module_id || "")?.manifest.name || job.input?.module_id || job.result?.module_id || ""}</h3>
          <div role="status"><Status value={job.status} /></div><JobProgress job={job} />
          {job.status === "failed" && <ErrorNotice error={{message: job.error_message, code: job.error_code}} message={u("jobFailed")} />}
          <div className="button-row">
            {canReadJobs && <button type="button" className="button secondary" onClick={() => onNavigate("/jobs#job=" + encodeURIComponent(job.id))}>{u("openJobs")}</button>}
            <span className="mono small">{job.id}</span>
          </div>
        </article>)}
      </div>
    </Panel> : <EmptyState>{u("moduleNoOperations")}</EmptyState>)}

    {section === "installed" && <Panel id="installed-modules" title={t("moduleInstalledList")}>
      <p className="ux-panel-description">{u("uxModulesInstalledHint")}</p>
      <div className="card-grid">
        {value.modules.map((module) => <article className="module-card" key={module.manifest.id} data-module-id={module.manifest.id}>
          <div className="module-title"><h2>{module.manifest.name}</h2>{module.status === "registered" ? <span className="tag">{u("incomplete")}</span> : <Status value={module.status} />}</div>
          <p>{module.manifest.description || t("noDescription")}</p>
          {module.error && <ErrorNotice error={{message: module.error}} message={module.status === "registered" ? u("resumeHint") : u("jobFailed")} />}
          {canManage && <div className="module-actions">
            <div className="button-row">
              {module.status === "registered" ? <button type="button" className="button primary" disabled={busy || Boolean(core.error)}
                onClick={() => void run(() => api.installModule(module.manifest))}>{u("retryInstall")}</button> : <>
                <button type="button" className="button secondary" disabled={busy || Boolean(core.error)}
                  onClick={() => void run(() => api.controlModule(module.manifest.id, module.status === "enabled" ? "disable" : "enable"))}>{t(module.status === "enabled" ? "moduleStop" : "moduleStart")}</button>
                <button type="button" className="button secondary" disabled={busy || Boolean(core.error)}
                  onClick={() => void run(() => api.controlModule(module.manifest.id, "restart"))}>{t("moduleRestart")}</button>
              </>}
            </div>
            <div className="button-row destructive-actions"><button type="button" className="button danger" disabled={busy || Boolean(core.error)} onClick={() => setConfirmRemove(module)}>{t("moduleRemove")}</button></div>
          </div>}
          <details className="technical-details"><summary>{u("details")}</summary>
            <dl className="details"><dt>ID</dt><dd className="mono">{module.manifest.id}</dd><dt>{t("version")}</dt><dd>{module.manifest.version}</dd>
              <dt>{t("coreVersion")}</dt><dd>{module.manifest.core}</dd><dt>{t("moduleImage")}</dt><dd className="mono">{module.manifest.runtime.docker.image}</dd>
            </dl>
          </details>
        </article>)}
        {value.modules.length === 0 && <EmptyState>{t("noModulesSimple")}</EmptyState>}
      </div>
    </Panel>}

    {section === "catalog" && <Panel id="module-catalog" title={t("moduleCatalogAvailable")} action={<button type="button" className="button secondary" disabled={catalog.loading} onClick={catalog.reload}>{u("refresh")}</button>}>
      <p className="ux-panel-description">{u("uxModulesCatalogHint")}</p>
      {catalog.loading && !catalog.data && <LoadingState />}
      {catalog.error && <ErrorState message={catalog.error} onRetry={catalog.reload} />}
      {catalog.data && <>
        <div className="catalog-summary"><p>{t("moduleCatalogOfficial")}</p><span className="mono small">{catalog.data.id}</span></div>
        <div className="card-grid">
          {catalog.data.modules.map((manifest) => {
            const existing = installed.get(manifest.id); const state = catalogState(existing, manifest);
            const hint = state === "newer" ? u("upgradeBlocked") : state === "older" ? u("installedNewer") : state === "changed" ? u("manifestChanged") : state === "invalid" ? u("versionUnknown") : "";
            const resumable = state === "incomplete" && existing;
            return <article className="module-card" key={manifest.id} data-catalog-id={manifest.id}>
              <div className="module-title"><h2>{manifest.name}</h2><span className="tag">{resumable ? u("incomplete") : existing ? t("moduleInstalled") : t("available")}</span></div>
              <p>{manifest.description || t("noDescription")}</p>
              <dl className="details"><dt>{t("availableVersion")}</dt><dd>{manifest.version}</dd>
                {existing && <><dt>{t("currentVersion")}</dt><dd>{existing.manifest.version}</dd></>}
                <dt>{t("coreVersion")}</dt><dd>{manifest.core}</dd></dl>
              {hint && <p className="notice">{hint}</p>}
              {resumable && <p>{u("resumeHint")}</p>}
              {canManage && <div className="button-row"><button type="button" className="button primary"
                disabled={busy || Boolean(core.error) || Boolean(existing && !resumable)}
                onClick={() => void run(() => resumable ? api.installModule(resumable.manifest) : api.installCatalogModule(manifest.id))}>
                {resumable ? u("retryInstall") : existing ? t("moduleInstalled") : runtime.data?.network_ready ? t("moduleInstall") : u("installAndPrepare")}
              </button></div>}
            </article>;
          })}
          {catalog.data.modules.length === 0 && <EmptyState>{t("moduleCatalogEmpty")}</EmptyState>}
        </div>
      </>}
    </Panel>}

    {section === "operations" && busy && displayedJobs.some(activeJob) && <p role="status" className="muted">{u("busyHint")}</p>}
    {section === "settings" && <details className="technical-details"><summary>{u("details")}</summary><Panel title={t("hostCapabilities")}><div className="tag-list">{value.capabilities.map((item) => <span key={item} className="tag">{item}</span>)}</div></Panel></details>}
    {section === "settings" && canManage && <details className="technical-details developer-install"><summary>{u("developer")}</summary>
      <Panel title={t("moduleInstallManifest")}><p id="manifest-hint">{u("manifestHint")}</p>
        <label htmlFor="module-manifest">{u("manifest")}</label>
        <textarea id="module-manifest" name="manifest" className="text-input mono" rows={12} value={manifestText}
          onChange={(event) => {setManifestText(event.target.value); setManifestError(false);}} placeholder={'{"schema_version":2,...}'}
          disabled={busy} spellCheck={false} aria-invalid={manifestError} aria-describedby={manifestError ? "manifest-hint manifest-error" : "manifest-hint"} />
        {manifestError && <div id="manifest-error" className="form-error" role="alert">{u("invalidJSON")}</div>}
        <div className="button-row"><button type="button" className="button primary" disabled={busy || !manifestText.trim() || Boolean(core.error)} onClick={installManifest}>{t("moduleInstall")}</button></div>
      </Panel>
    </details>}
    {confirmRemove && <ConfirmDialog title={u("removeTitle")} confirmLabel={u("confirmRemove")} onCancel={() => setConfirmRemove(undefined)}
      onConfirm={() => {void run(() => api.removeModule(confirmRemove.manifest.id)); setConfirmRemove(undefined);}}>
      <p><strong>{confirmRemove.manifest.name}</strong></p><p>{u("removeHint")}</p>
    </ConfirmDialog>}
  </div>;
}
