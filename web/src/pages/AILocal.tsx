import {useCallback, useEffect, useRef, useState} from "react";
import {api} from "../api/client";
import type {AIHardware, AILocalModels, AILocalStatus, AIGPUAssignment} from "../api/types";
import {Panel} from "../components/Panel";

const gib = (bytes: number | undefined) => bytes ? (bytes / (1024 ** 3)).toFixed(1) + " ГБ" : "Не определено";
const modelText = (value: unknown) => value instanceof Error ? value.message : String(value);

export function AILocalPage({onNewGPU, onModelsReady, onOpenChat, aiVersion}: {
  aiVersion?: string;
  onNewGPU: (names: string[]) => void;
  onModelsReady: () => void;
  onOpenChat: () => void;
}) {
  const [hardware, setHardware] = useState<AIHardware>();
  const [local, setLocal] = useState<AILocalStatus>();
  const [models, setModels] = useState<AILocalModels>();
  const [assignment, setAssignment] = useState<AIGPUAssignment>();
  const [gpuPackageReady, setGpuPackageReady] = useState<boolean | undefined>();
  const [gpuDownloadQueued, setGpuDownloadQueued] = useState(false);
  const splitGPURuntime = Boolean(aiVersion && (() => {
    const numbers = aiVersion.split(".").map((part) => Number(part));
    return numbers.length === 3 && numbers.every(Number.isFinite) &&
      (numbers[0] > 0 || numbers[1] >= 3);
  })());
  const [gpuNotice, setGpuNotice] = useState("");
  const seenAssignment = useRef<string | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const observed = useRef<Set<string> | null>(null);
  const previousReady = useRef(false);
  const newGPURef = useRef(onNewGPU);
  const readyRef = useRef(onModelsReady);
  useEffect(() => {newGPURef.current = onNewGPU; readyRef.current = onModelsReady;}, [onNewGPU, onModelsReady]);

  const refresh = useCallback(async () => {
    // Existing 0.1.0 AI images do not yet implement /api/local/*.
    // Hardware must remain visible even if Ollama is unavailable.
    const [resource, engine, catalog, allocation, packageStatus] = await Promise.allSettled([
      api.aiHardware(), api.aiLocalStatus(), api.aiLocalModels(), api.aiGPUAssignment(),
      splitGPURuntime ? api.aiGPUImageStatus() : Promise.resolve(undefined),
    ]);
    if (resource.status === "fulfilled") {
      const inventory = resource.value;
      setHardware(inventory);
      const devices = inventory.gpus || [];
      const now = new Set(devices.map((gpu) => gpu.pci_address || gpu.model || "").filter(Boolean));
      if (observed.current !== null) {
        const added = devices.filter((gpu) => {
          const key = gpu.pci_address || gpu.model || "";
          return key && !observed.current?.has(key);
        });
        if (added.length > 0) newGPURef.current(added.map((gpu) => gpu.model || gpu.pci_address || "GPU"));
      }
      observed.current = now;
      setSelected((values) => values.filter((id) => now.has(id)));
    }
    if (engine.status === "fulfilled") {
      setLocal(engine.value);
      const ready = engine.value.engine_state === "ready" &&
        engine.value.model_state === "ready";
      if (ready && !previousReady.current) readyRef.current();
      previousReady.current = ready;
    }
    if (catalog.status === "fulfilled") setModels(catalog.value);
    if (packageStatus.status === "fulfilled" && packageStatus.value) {
      setGpuPackageReady(packageStatus.value.ready);
      if (packageStatus.value.ready) setGpuDownloadQueued(false);
    }
    if (allocation.status === "fulfilled") {
      setAssignment(allocation.value);
      const fingerprint = [...allocation.value.assigned_gpu_pci].sort().join(",");
      if (seenAssignment.current !== fingerprint) {
        seenAssignment.current = fingerprint;
        setSelected(allocation.value.assigned_gpu_pci);
      }
    }
    if (resource.status === "rejected") setError("Не удалось получить ресурсы сервера");
    else if (engine.status === "rejected") setError("Для встроенного Ollama требуется обновление AI-модуля");
    else setError("");
    setLoading(false);
  }, [splitGPURuntime]);

  useEffect(() => {
    let live = true;
    // Refresh only while mounted, no recurrent model/provider action.
    const run = () => {
      if (live) void refresh().catch(() => {if (live) setError("Не удалось обновить состояние локального AI");});
    };
    run();
    const timer = window.setInterval(run, 10000);
    return () => {live = false; window.clearInterval(timer);};
  }, [refresh]);

  async function download(name: string) {
    if (busy) return;
    const preset = models?.recommended_models?.find((item) => item.name === name);
    if (!preset) return;
    const message = "Загрузить " + name + " (~" + preset.size_gb + " ГБ)? Модель сохраняется на сервере.";
    if (!window.confirm(message)) return;
    setBusy(true); setError("");
    try {
      await api.aiPullLocalModel(name);
      await refresh();
    } catch (reason) {setError(modelText(reason));}
    finally {setBusy(false);}
  }

  async function prepareGPU() {
    if (busy || gpuDownloadQueued) return;
    if (!window.confirm("Загрузить отдельный полный GPU-образ Ollama? Это большой пакет. " +
      "AI продолжит работать на CPU; после загрузки нужно отдельно подтвердить включение GPU.")) return;
    setBusy(true);
    setError("");
    try {
      await api.aiPrepareGPUImage();
      setGpuDownloadQueued(true);
      setGpuNotice("Загрузка GPU-компонентов запущена. Ход загрузки — «Модули → Операции».");
    } catch (reason) {
      setError(modelText(reason));
    } finally {
      setBusy(false);
    }
  }

  async function applyGPU() {
    if (busy || !assignment) return;
    const chosen = selected.filter((id) => hardware?.gpus.some((gpu) =>
      gpu.pci_address === id && ((gpu.vendor_id || "").toLowerCase() === "0x10de" ||
        (gpu.vendor || "").toLowerCase().includes("nvidia")) && gpu.driver === "nvidia"));
    if (splitGPURuntime && chosen.length > 0 && gpuPackageReady !== true) {
      setError("Сначала загрузите GPU-компоненты и дождитесь завершения в разделе «Модули → Операции».");
      return;
    }
    if (chosen.length !== selected.length) {
      setError("Для использования доступны только GPU NVIDIA с работающим драйвером.");
      return;
    }
    if (!window.confirm("Изменить набор GPU для Ollama? AI Agent кратковременно перезапустится. " +
      "Текущие сообщения могут быть прерваны; данные и загруженные модели сохраняются.")) return;
    setBusy(true);
    setError("");
    setGpuNotice("");
    try {
      const result = await api.aiApplyGPU(chosen);
      setAssignment(result);
      seenAssignment.current = [...result.assigned_gpu_pci].sort().join(",");
      setGpuNotice(result.mode === "cpu" ? "Возврат к CPU подтверждён." :
        "GPU назначены контейнеру: " + result.assigned_gpu_pci.length + ". Проверьте ускорение на своей модели.");
    } catch (reason) {
      setError("Операция не подтверждена. Проверьте текущий режим перед повтором. " + modelText(reason));
    } finally {
      setBusy(false);
      void refresh();
    }
  }

  const recommended = models?.recommended_models || local?.recommended_models || [];
  const installed = new Set(models?.installed || []);
  const gpus = hardware?.gpus || [];
  const isBusy = busy || local?.model_state === "downloading";
  const availableRAMGiB = (hardware?.memory.available_bytes || 0) / (1024 ** 3);
  const fittingModels = recommended.filter((item) => item.minimum_ram_gb <= availableRAMGiB);
  const suggestedModel = fittingModels.length ? fittingModels[fittingModels.length - 1].name : "qwen3:0.6b";
  const nvidiaCandidates = gpus.filter((gpu) =>
    ((gpu.vendor_id || "").toLowerCase() === "0x10de" ||
      (gpu.vendor || "").toLowerCase().includes("nvidia")) && gpu.driver === "nvidia");


  return <div className="ai-local-page">
    {error && <p role="alert" className="ai-error">{error}</p>}
    {hardware && <Panel title="Автоматический анализ ресурсов">
      <p>Сервер: <strong>{hardware.cpu.logical_cpus}</strong> потоков CPU,
        доступная ОЗУ <strong>{gib(hardware.memory.available_bytes)}</strong>.
        Видеокарт обнаружено: <strong>{gpus.length}</strong>.</p>
      <p>По объёму доступной оперативной памяти рекомендуем модель
        <strong> {suggestedModel}</strong> (консервативная оценка для CPU).
        Базовая qwen3:0.6b запускается первой независимо от доступности GPU.</p>
      {nvidiaCandidates.length > 0
        ? <p>Есть {nvidiaCandidates.length} NVIDIA GPU с драйвером.
            Их можно выбрать ниже и подтвердить назначение для Ollama; автоматического
            захвата ресурса не происходит.</p>
        : <p>Пока доступен CPU-режим. Подключённая GPU появится после обнаружения её ОС.</p>}
      <p className="muted">Предварительная рекомендация основана на объёме ОЗУ, а не на замере VRAM
        или производительности: перед выбором крупной модели проверьте свободную память и диск.</p>
    </Panel>}

    <Panel title="Встроенный Ollama">
      <p className="muted">AI Agent автоматически запускает локальную модель без облачного ключа.
        Первый запуск требует загрузки файлов модели; после этого можно работать без Интернета.</p>
      {loading && !local && <p role="status">Проверяем Ollama и ресурсы сервера…</p>}
      {local && <div className="ai-local-status" role="status">
        <span>Движок: <strong>{local.engine_state === "ready" ? "запущен" :
          local.engine_state === "starting" ? "запускается" :
          local.engine_state === "error" ? "ошибка" : "отключён"}</strong></span>
        <span>Модель: <strong>{local.active_model || "ожидание"}</strong></span>
        <span>Состояние: <strong>{local.model_state === "ready" ? "готова" :
          local.model_state === "downloading" ? "загружается" :
          local.model_state === "error" ? "ошибка загрузки" : "ожидание"}</strong></span>
      </div>}
      {local?.model_state === "downloading" && <div className="ai-model-progress">
        <p>Загрузка модели: {local.download_status || "ожидание сервера"}</p>
        <progress value={local.download_percent ?? undefined} max="100" aria-label="Загрузка локальной модели" />
        {local.download_percent !== null && local.download_percent !== undefined &&
          <small>{local.download_percent}%</small>}
      </div>}
      {local?.error && <p className="ai-error" role="alert">{local.error}</p>}
      {local?.model_state === "ready" &&
        <button type="button" className="button primary" onClick={onOpenChat}>Перейти к чату</button>}
    </Panel>

    <Panel title="Установленные и доступные модели">
      <p>Установлено: <strong>{installed.size}</strong>. Новые модели загружаются только после подтверждения;
        одновременно допускается одна загрузка.</p>
      <div className="ai-model-list">
        {recommended.map((item) => <article className="ai-model-entry" key={item.name}>
          <div>
            <strong>{item.name}</strong>
            <span>{item.purpose} · ≈ {item.size_gb} ГБ</span>
            <small>{item.recommended ? "Подходит по объёму ОЗУ" : "Может потребовать больше ОЗУ"}</small>
          </div>
          {installed.has(item.name) ?
            <span className="tag">Установлена</span> :
            <button type="button" className="button secondary" disabled={isBusy || local?.engine_state !== "ready"}
              onClick={() => {void download(item.name);}}>Загрузить</button>}
        </article>)}
      </div>
    </Panel>

    <Panel title="Ресурсы сервера и видеокарты">
      {gpuNotice && <p className="ai-notice" role="status">{gpuNotice}</p>}
      <div className="button-row">
        <button type="button" className="button secondary" onClick={() => {void refresh();}}>
          Проверить оборудование
        </button>
      </div>
      {hardware && <div className="ai-hardware-facts">
        <span>CPU: <strong>{hardware.cpu.model || "Сервер"} ({hardware.cpu.logical_cpus} потоков)</strong></span>
        <span>Память: <strong>{gib(hardware.memory.available_bytes)} доступно из {gib(hardware.memory.total_bytes)}</strong></span>
        <span>Видеокарты: <strong>{gpus.length}</strong></span>
      </div>}
      {!hardware && <p className="muted">Ожидание сведений ядра о процессоре и видеокартах…</p>}
      {!gpus.length && hardware && <p>GPU не обнаружены операционной системой. Агент работает через CPU.</p>}
      {!!gpus.length && <div className="ai-gpu-list">
        {gpus.map((gpu, index) => {
          const id = gpu.pci_address || gpu.model || String(index);
          const eligible = ((gpu.vendor_id || "").toLowerCase() === "0x10de" ||
            (gpu.vendor || "").toLowerCase().includes("nvidia")) && gpu.driver === "nvidia";
          return <label key={id} className="ai-gpu-device">
            <input type="checkbox" checked={selected.includes(id)} disabled={!eligible || busy || !assignment}
              onChange={(event) => {
              setSelected((current) => event.target.checked
                ? [...current, id] : current.filter((v) => v !== id));
            }}/>
            <span><strong>{gpu.model || gpu.vendor || "GPU"}</strong>
              <small>{gpu.vendor || "Производитель не определён"}
                {gpu.pci_address ? " · PCI " + gpu.pci_address : ""}
                {gpu.driver ? " · драйвер " + gpu.driver : " · драйвер не определён"}
                {!eligible ? " · ускорение ещё не поддерживается" : ""}</small></span>
          </label>;
        })}
      </div>}
      {assignment ? <p className="ai-gpu-choice" role="status">
        Текущий режим: <strong>{assignment.mode === "cpu" ? "CPU" : "NVIDIA GPU"}</strong>,
        назначено карт: <strong>{assignment.assigned_gpu_pci.length}</strong>.
        Выбрано: <strong>{selected.length}</strong> из {gpus.length}.
      </p> : <p className="muted">Назначение GPU недоступно: требуется новый привилегированный helper.
        Видеокарты обнаружены, но их использование пока не подтверждено.</p>}
      {splitGPURuntime && nvidiaCandidates.length > 0 && <div className="ai-note" role="status">
        GPU-ускорение использует отдельный полный Docker-образ. Он не загружается
        при обычной установке AI Agent.
        <p>GPU-компоненты: <strong>{gpuPackageReady === true ? "загружены" :
          gpuDownloadQueued ? "загружаются" :
          gpuPackageReady === false ? "не загружены" : "состояние неизвестно"}</strong>.</p>
        {gpuPackageReady === false && <button type="button" className="button secondary"
          disabled={busy || gpuDownloadQueued}
          onClick={() => {void prepareGPU();}}>
          {gpuDownloadQueued ? "Загрузка запущена" : "Загрузить GPU-компоненты"}
        </button>}
        {gpuDownloadQueued && <p>Ход скачивания виден на странице «Модули → Операции».
          После завершения вернитесь и подтвердите назначение выбранных GPU.</p>}
      </div>}
      {assignment && <div className="button-row">
        <button type="button" className="button primary" disabled={busy ||
          (splitGPURuntime && selected.length > 0 && gpuPackageReady !== true) ||
          [...selected].sort().join(",") === [...assignment.assigned_gpu_pci].sort().join(",")}
          onClick={() => {void applyGPU();}}>
          {selected.length ? "Применить выбранные GPU" : "Переключить на CPU"}
        </button>
      </div>}
      <p className="muted">Список оборудования проверяется каждые 10 секунд.
        Для GPU нужны совместимый драйвер и NVIDIA Container Toolkit. Можно назначить
        несколько NVIDIA GPU одновременно; Ollama решает, как распределять загрузку.
        Устройства AMD/Intel отображаются, но их выдача контейнеру пока не реализована.
        При неудаче нового контейнера Core пытается восстановить прежний режим.
        До явного подтверждения новые GPU автоматически не подключаются.</p>
    </Panel>
  </div>;
}
