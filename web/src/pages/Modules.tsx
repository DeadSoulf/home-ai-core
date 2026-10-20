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
import { activeJob, canUpgradeModule, catalogState, jobTitle, runtimeMessage } from "../ui/moduleState";
import { PageHeading, Status } from "./Dashboard";
import { homeAssistantURL } from "../ui/homeAssistant";

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
  const [confirmUpgrade, setConfirmUpgrade] = useState<RegisteredModule>();
  const [confirmNVRStorage, setConfirmNVRStorage] = useState<RegisteredModule>();
  const [confirmHomeAssistant, setConfirmHomeAssistant] = useState<() => Promise<Job>>();
  const mutation = useRef(false);
  // Catalog outages never hide the installed registry or stop/remove controls.
  const load = useCallback(() => Promise.all([api.modules(), api.capabilities()]).then(([modules, capabilities]) => ({modules, capabilities})), []);
  const core = useResource(load, revision);
  // Each page loads only what it needs. Installed controls remain usable during catalog outages.
  const loadCatalog = useCallback(() => section === "catalog" || section === "installed" ? api.moduleCatalog() : Promise.resolve(undefined), [section]);
  const catalog = useResource(loadCatalog);
  const loadRuntime = useCallback((signal: AbortSignal) =>
    section === "catalog" || section === "settings" ? api.moduleRuntime(signal) : Promise.resolve(undefined), [section]);
  const runtime = useResource(loadRuntime, 0, section === "settings" ? 15000 : 0);
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
    if (!canManage || busy || mutation.current || !core.data || core.error) return;
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

  function requestInstall(manifest: RegisteredModule["manifest"], action: () => Promise<Job>) {
    if (manifest.id === "homeassistant.module") setConfirmHomeAssistant(() => action);
    else void run(action);
  }

  function installManifest() {
    setManifestError(false);
    try {
      if (new TextEncoder().encode(manifestText).length > 256 * 1024) throw new Error("too large");
      const manifest = JSON.parse(manifestText) as RegisteredModule["manifest"];
      if (!manifest || typeof manifest !== "object" || Array.isArray(manifest) || typeof manifest.id !== "string" || !manifest.id.trim()) throw new Error("invalid object");
      requestInstall(manifest, () => api.installModule(manifest));
    } catch { setManifestError(true); document.getElementById("module-manifest")?.focus(); }
  }

  const value = core.data ?? {modules: [], capabilities: []};
  const installed = new Map(value.modules.map((item) => [item.manifest.id, item]));
  // Include recent finished operations so a page reload does not erase the
  // answer to "was the image installed, failed, or is it still downloading?".
  const displayedJobs = [...(operations.data || [])];
  if (lastJob && !displayedJobs.some((job) => job.id === lastJob.id)) displayedJobs.unshift(lastJob);
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
    {core.loading && !core.data && <LoadingState />}
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
    {catalog.data?.homeassistant_status === "upstream_unavailable" && <p className="notice" role="status">Проверка новых релизов Home Assistant временно недоступна. Доступна закреплённая версия из каталога; установленный контейнер не изменён.</p>}
    {catalog.data?.homeassistant_status === "digest_changed" && <p className="notice" role="status">Digest уже опубликованной версии Home Assistant изменился. Автоматическая подмена отклонена; используется проверенный образ каталога.</p>}
    {section === "operations" && (displayedJobs.length > 0 ? <Panel title={u("uxOperationsJump")}>
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

    {section === "installed" && core.data && <Panel id="installed-modules" title={t("moduleInstalledList")}
      action={<button type="button" className="button secondary" disabled={catalog.loading || busy} onClick={catalog.reload}>{u("moduleUpgradeCheck")}</button>}>
      <p className="ux-panel-description">{u("uxModulesInstalledHint")}</p>
      {catalog.error && <p className="notice" role="status">{u("moduleUpgradeCatalogError")}</p>}
      {catalog.data?.private_nvr_status === "missing_token" &&
        <p className="notice" role="status">Закрытое обновление NVR 1.1.6 доступно владельцу, но не настроен ключ доступа GHCR. Стабильная публичная версия остаётся доступной.</p>}
      {catalog.data?.private_nvr_status === "unavailable" &&
        <p className="notice" role="status">Закрытое обновление NVR недоступно: проверьте токен с правами чтения приватного репозитория и пакетов GHCR.</p>}
      <div className="card-grid">
        {value.modules.map((module) => {
          const candidate = catalog.data?.modules.find((item) => item.id === module.manifest.id);
          const state = candidate ? catalogState(module, candidate) : undefined;
          const upgradeReady = !catalog.error && !catalog.loading && !core.error && canUpgradeModule(module, candidate);
          const upgradeStatus = catalog.error ? u("moduleUpgradeCatalogError") :
            catalog.loading || !catalog.data ? u("moduleUpgradeChecking") :
            !candidate ? u("moduleUpgradeNotListed") :
            state === "newer" && candidate.runtime.docker.image === module.manifest.runtime.docker.image ? u("moduleUpgradeSameImage") :
            state === "newer" && (!module.manifest.lifecycle.includes("upgrade") || !candidate.lifecycle.includes("upgrade")) ? u("moduleUpgradeNotSupported") :
            state === "newer" ? (module.status === "enabled" ? u("moduleUpgradeAvailable") : u("moduleUpgradeDisabled")) :
            state === "installed" ? u("moduleUpgradeUpToDate") :
            state === "changed" ? u("manifestChanged") :
            state === "older" ? u("installedNewer") :
            state === "incomplete" ? u("resumeHint") : u("versionUnknown");
          return <article className="module-card" key={module.manifest.id} data-module-id={module.manifest.id}>
          <div className="module-title"><h2>{module.manifest.name}</h2>{module.status === "registered" ? <span className="tag">{u("incomplete")}</span> : <Status value={module.status} />}</div>
          <p>{module.manifest.description || t("noDescription")}</p>
          {module.manifest.id === "homeassistant.module" && module.status === "enabled" && <div className="button-row">
            <a className="button secondary" href={homeAssistantURL(window.location.href)} target="_blank" rel="noopener noreferrer">Открыть Home Assistant</a>
            <span className="small muted">Порт 8123. У Home Assistant отдельная учётная запись.</span>
          </div>}
          {module.error && <ErrorNotice error={{message: module.error}} message={module.status === "registered" ? u("resumeHint") : u("jobFailed")} />}
          {canManage && <div className="module-actions">
            <div className="button-row">
              {module.status === "registered" ? <button type="button" className="button primary" disabled={busy || (!core.data || Boolean(core.error))}
                onClick={() => requestInstall(module.manifest, () => api.installModule(module.manifest))}>{u("retryInstall")}</button> : <>
                <button type="button" className="button secondary" disabled={busy || (!core.data || Boolean(core.error))}
                  onClick={() => void run(() => api.controlModule(module.manifest.id, module.status === "enabled" ? "disable" : "enable"))}>{t(module.status === "enabled" ? "moduleStop" : "moduleStart")}</button>
                <button type="button" className="button secondary" disabled={busy || (!core.data || Boolean(core.error))}
                  onClick={() => void run(() => api.controlModule(module.manifest.id, "restart"))}>{t("moduleRestart")}</button>
                {module.manifest.id === "nvr.module" && module.status === "enabled" &&
                  <button type="button" className="button secondary" disabled={busy || (!core.data || Boolean(core.error))}
                    onClick={() => setConfirmNVRStorage(module)}>Подключить назначенный диск NVR</button>}
              </>}
            </div>
            <div className="module-upgrade-controls">
              <div className="button-row">
                <button type="button" className="button secondary" disabled={catalog.loading || busy}
                  onClick={catalog.reload}>{catalog.loading ? u("refreshing") : u("moduleUpgradeCheck")}</button>
                <button type="button" className="button primary" data-module-upgrade={module.manifest.id}
                  disabled={busy || !upgradeReady} onClick={() => setConfirmUpgrade(module)}>{u("moduleUpgradeButton")}</button>
              </div>
              <p role="status" className="small muted">
                {upgradeStatus}
                {candidate && state === "newer" && <> {t("currentVersion")}: {module.manifest.version} → {t("availableVersion")}: {candidate.version}</>}
              </p>
            </div>
            <div className="button-row destructive-actions"><button type="button" className="button danger" disabled={busy || (!core.data || Boolean(core.error))} onClick={() => setConfirmRemove(module)}>{t("moduleRemove")}</button></div>
          </div>}
          <details className="technical-details"><summary>{u("details")}</summary>
            <dl className="details"><dt>ID</dt><dd className="mono">{module.manifest.id}</dd><dt>{t("version")}</dt><dd>{module.manifest.version}</dd>
              <dt>{t("coreVersion")}</dt><dd>{module.manifest.core}</dd><dt>{t("moduleImage")}</dt><dd className="mono">{module.manifest.runtime.docker.image}</dd>
            </dl>
          </details>
        </article>;
        })}
        {value.modules.length === 0 && <EmptyState>{t("noModulesSimple")}</EmptyState>}
      </div>
    </Panel>}

    {section === "catalog" && <Panel id="module-catalog" title={t("moduleCatalogAvailable")} action={<button type="button" className="button secondary" disabled={catalog.loading} onClick={catalog.reload}>{u("refresh")}</button>}>
      <p className="ux-panel-description">{u("uxModulesCatalogHint")}</p>
      {catalog.loading && !catalog.data && <LoadingState />}
      {catalog.error && <ErrorState message={catalog.error} onRetry={catalog.reload} />}
      {catalog.data && <>
        <div className="catalog-summary"><p>{t("moduleCatalogOfficial")}</p><span className="mono small">{catalog.data.id}</span></div>
        {catalog.data.private_nvr_status === "available" && <p className="notice" role="status">NVR: доступна закрытая версия HCNetSDK 6.1.11.40. Перед обновлением проверьте диск NVR и сохранность архива.</p>}
        {catalog.data.private_nvr_status === "missing_token" && <p className="notice" role="status">Для закрытого обновления NVR требуется настроить HOME_AI_MODULE_CATALOG_TOKEN на Core и updater. Пока доступна публичная версия.</p>}
        {catalog.data.private_nvr_status === "unavailable" && <p className="notice" role="status">Закрытый каталог NVR недоступен по текущим учётным данным GitHub/GHCR.</p>}
        <div className="card-grid">
          {catalog.data.modules.map((manifest) => {
            const existing = installed.get(manifest.id); const state = catalogState(existing, manifest);
            const hint = state === "newer" ?
              !existing ? "" :
              manifest.runtime.docker.image === existing.manifest.runtime.docker.image ? u("moduleUpgradeSameImage") :
              !existing.manifest.lifecycle.includes("upgrade") || !manifest.lifecycle.includes("upgrade") ? u("moduleUpgradeNotSupported") :
              existing.status !== "enabled" ? u("moduleUpgradeDisabled") :
              u("moduleUpgradeAvailable") :
              state === "older" ? u("installedNewer") :
              state === "changed" ? u("manifestChanged") :
              state === "invalid" ? u("versionUnknown") :
              state === "installed" && existing ? u("moduleUpgradeUpToDate") : "";
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
                disabled={busy || (!core.data || Boolean(core.error)) || Boolean(existing && !resumable)}
                onClick={() => requestInstall(resumable ? resumable.manifest : manifest, () => resumable ? api.installModule(resumable.manifest) : api.installCatalogModule(manifest.id))}>
                {resumable ? u("retryInstall") : existing ? t("moduleInstalled") : runtime.data?.network_ready ? t("moduleInstall") : u("installAndPrepare")}
              </button>
                {existing && <button type="button"
                  className="button primary" data-catalog-upgrade={manifest.id}
                  disabled={busy || catalog.loading || Boolean(catalog.error) || !canUpgradeModule(existing, manifest)}
                  onClick={() => setConfirmUpgrade(existing)}>{u("moduleUpgradeButton")}</button>}
              </div>}
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
        <div className="button-row"><button type="button" className="button primary" disabled={busy || !manifestText.trim() || (!core.data || Boolean(core.error))} onClick={installManifest}>{t("moduleInstall")}</button></div>
      </Panel>
    </details>}
    {confirmHomeAssistant && <ConfirmDialog title="Установка Home Assistant" confirmLabel="Разрешить и установить"
      onCancel={() => setConfirmHomeAssistant(undefined)} onConfirm={() => {
        const action = confirmHomeAssistant;
        setConfirmHomeAssistant(undefined);
        void run(action);
      }}>
      <p>Официальный контейнер получит сеть сервера (host), порт 8123 и расширенные права (privileged), включая доступ к устройствам. Это не изолированный профиль обычного модуля.</p>
      <p>Конфигурация хранится отдельно и сохраняется при удалении. D-Bus подключается только при наличии. Docker socket контейнеру не передаётся.</p>
      <p>Доступ к порту 8123 ограничьте доверенной локальной сетью. Supervisor и магазин приложений Home Assistant OS не входят в Container.</p>
    </ConfirmDialog>}
    {confirmNVRStorage && <ConfirmDialog title="Перенос NVR на назначенный диск" confirmLabel="Перенести и проверить"
      onCancel={() => setConfirmNVRStorage(undefined)} onConfirm={() => {
        void run(() => api.controlModule("nvr.module", "activate_storage"));
        setConfirmNVRStorage(undefined);
      }}>
      <p>Запись камер будет временно остановлена. HomeAiCore скопирует базу камер и архив на диск, назначенный в «Файлы → Хранилище», проверит целостность данных и работу NVR.</p>
      <p>Старые файлы сохраняются как резервная копия. Если проверка не пройдёт, будет выполнена попытка отката.</p>
    </ConfirmDialog>}
    {confirmUpgrade && <ConfirmDialog title={u("moduleUpgradeTitle")} confirmLabel={u("moduleUpgradeButton")}
      onCancel={() => setConfirmUpgrade(undefined)}
      onConfirm={() => {
        const id = confirmUpgrade.manifest.id;
        setConfirmUpgrade(undefined);
        void run(() => api.upgradeCatalogModule(id));
      }}>
      <p><strong>{confirmUpgrade.manifest.name}</strong></p>
      {confirmUpgrade.manifest.id === "homeassistant.module"
        ? <p>Home Assistant будет остановлен для копирования всей конфигурации. Новая версия запустится с отдельной копией и пройдёт HTTP-проверку. При ошибке запуска будет выполнена попытка вернуть предыдущий контейнер с неизменённой исходной конфигурацией. Требуется свободное место для полной копии.</p>
        : <p>{u("moduleUpgradeHint")}</p>}
    </ConfirmDialog>}
    {confirmRemove && <ConfirmDialog title={u("removeTitle")} confirmLabel={u("confirmRemove")} onCancel={() => setConfirmRemove(undefined)}
      onConfirm={() => {void run(() => api.removeModule(confirmRemove.manifest.id)); setConfirmRemove(undefined);}}>
      <p><strong>{confirmRemove.manifest.name}</strong></p><p>{u("removeHint")}</p>
    </ConfirmDialog>}
  </div>;
}
