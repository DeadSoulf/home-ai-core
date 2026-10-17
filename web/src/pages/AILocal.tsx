import {useCallback, useEffect, useRef, useState} from "react";
import {api} from "../api/client";
import type {AIHardware, AILocalModels, AILocalStatus} from "../api/types";
import {Panel} from "../components/Panel";

const gib = (bytes: number | undefined) => bytes ? (bytes / (1024 ** 3)).toFixed(1) + " ГБ" : "Не определено";
const modelText = (value: unknown) => value instanceof Error ? value.message : String(value);

export function AILocalPage({onNewGPU, onModelsReady, onOpenChat}: {
  onNewGPU: (names: string[]) => void;
  onModelsReady: () => void;
  onOpenChat: () => void;
}) {
  const [hardware, setHardware] = useState<AIHardware>();
  const [local, setLocal] = useState<AILocalStatus>();
  const [models, setModels] = useState<AILocalModels>();
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
    const [resource, engine, catalog] = await Promise.allSettled([
      api.aiHardware(), api.aiLocalStatus(), api.aiLocalModels(),
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
    if (resource.status === "rejected") setError("Не удалось получить ресурсы сервера");
    else if (engine.status === "rejected") setError("Для встроенного Ollama требуется обновление AI-модуля");
    else setError("");
    setLoading(false);
  }, []);

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

  const recommended = models?.recommended_models || local?.recommended_models || [];
  const installed = new Set(models?.installed || []);
  const gpus = hardware?.gpus || [];
  const isBusy = busy || local?.model_state === "downloading";

  return <div className="ai-local-page">
    {error && <p role="alert" className="ai-error">{error}</p>}
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
          return <label key={id} className="ai-gpu-device">
            <input type="checkbox" checked={selected.includes(id)} onChange={(event) => {
              setSelected((current) => event.target.checked
                ? [...current, id] : current.filter((v) => v !== id));
            }}/>
            <span><strong>{gpu.model || gpu.vendor || "GPU"}</strong>
              <small>{gpu.vendor || "Производитель не определён"}
                {gpu.pci_address ? " · PCI " + gpu.pci_address : ""}
                {gpu.driver ? " · драйвер " + gpu.driver : " · драйвер не определён"}</small></span>
          </label>;
        })}
      </div>}
      {gpus.length > 0 && <p className="ai-gpu-choice">
        Выбрано GPU: <strong>{selected.length}</strong> из {gpus.length}.
        Встроенный Ollama сейчас запускается в режиме CPU. Сами отметки не подключают
        устройства к Docker-контейнеру.
      </p>}
      <p className="muted">Список обновляется каждые 10 секунд: новая PCI-видеокарта появится после
        её обнаружения ОС. Для ускорения через NVIDIA нужен рабочий драйвер,
        NVIDIA Container Toolkit и отдельное подтверждённое назначение устройств;
        простое обнаружение ещё не означает возможность GPU-вычислений.
        Подключение нескольких GPU будет выполняться через защищённый runtime ядра.</p>
    </Panel>
  </div>;
}
