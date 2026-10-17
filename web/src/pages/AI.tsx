import {useCallback, useEffect, useState, type FormEvent} from "react";
import {api} from "../api/client";
import type {AIApproval, AIConversation, AIMessage, AIProvider, AIStatus} from "../api/types";
import {Panel} from "../components/Panel";
import {PageHeading} from "./Dashboard";
import {AILocalPage} from "./AILocal";
import "./AI.css";

type AITab = "chat" | "local" | "providers" | "approvals";
const messageOf = (error: unknown) => error instanceof Error ? error.message : String(error);
const tabNames: Record<AITab, string> = {
  chat: "Чат", local: "Локальный AI", providers: "AI-провайдеры", approvals: "Подтверждения",
};

export function AIPage() {
  const [tab, setTab] = useState<AITab>("chat");
  const [status, setStatus] = useState<AIStatus>();
  const [providers, setProviders] = useState<AIProvider[]>([]);
  const [conversations, setConversations] = useState<AIConversation[]>([]);
  const [approvals, setApprovals] = useState<AIApproval[]>([]);
  const [messages, setMessages] = useState<AIMessage[]>([]);
  const [modelId, setModelId] = useState("");
  const [conversationId, setConversationId] = useState("");
  const [draft, setDraft] = useState("");
  const [name, setName] = useState("");
  const [kind, setKind] = useState<AIProvider["kind"]>("ollama");
  const [baseURL, setBaseURL] = useState("");
  const [model, setModel] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [gpuNotice, setGpuNotice] = useState<string[]>([]);

  const reload = useCallback(async () => {
    const [nextStatus, nextProviders, nextConversations, nextApprovals] = await Promise.all([
      api.aiStatus(), api.aiProviders(), api.aiConversations(), api.aiApprovals(),
    ]);
    setStatus(nextStatus);
    setProviders(nextProviders);
    setConversations(nextConversations);
    setApprovals(nextApprovals);
    setModelId((current) => nextProviders.some((item) => item.id === current)
      ? current : nextProviders[0]?.id || "");
  }, []);

  useEffect(() => {
    let active = true;
    void reload().catch((err: unknown) => {
      if (active) setError(messageOf(err));
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [reload]);

  useEffect(() => {
    if (!conversationId) {
      setMessages([]);
      return;
    }
    let active = true;
    void api.aiMessages(conversationId)
      .then((result) => { if (active) setMessages(result); })
      .catch((err: unknown) => { if (active) setError(messageOf(err)); });
    return () => { active = false; };
  }, [conversationId]);

  async function run(action: () => Promise<unknown>, success: string) {
    if (busy) return false;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      await reload();
      setNotice(success);
      return true;
    } catch (err) {
      setError(messageOf(err));
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const text = draft.trim();
    if (!text || !modelId || busy) return;
    setBusy(true); setError(""); setNotice("");
    try {
      const result = await api.aiChat(modelId, text, conversationId || undefined);
      setConversationId(result.conversation_id);
      setDraft("");
      await reload();
      // Reusing the current conversation does not retrigger its ID effect.
      setMessages(await api.aiMessages(result.conversation_id));
      if (result.approval_ids.length) setNotice("Агент предложил действие. Проверьте вкладку «Подтверждения».");
    } catch (err) {
      setError(messageOf(err));
      await reload().catch(() => {});
    } finally {
      setBusy(false);
    }
  }

  async function addProvider(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const ok = await run(() => api.aiAddProvider({
      name: name.trim(), kind, base_url: baseURL.trim(),
      model: model.trim(), api_key: apiKey,
    }), "Модель добавлена.");
    if (ok) {
      setApiKey(""); setName(""); setModel(""); setTab("chat");
    }
  }

  return <div className="page ai-page">
    <PageHeading title="AI Agent" subtitle="Локальные модели и внешние AI — через защищённый интерфейс HomeAICore" />
    <nav className="ai-tabs" aria-label="Разделы AI Agent">
      {(["chat", "local", "providers", "approvals"] as const).map((item) =>
        <button key={item} type="button" className={"button " + (tab === item ? "primary" : "secondary")}
          aria-current={tab === item ? "page" : undefined}
          onClick={() => {setTab(item); setError(""); setNotice("");}}>
          {tabNames[item]}{item === "approvals" && status ? ` (${status.approvals_pending})` : ""}
        </button>)}
      <button type="button" className="button secondary" disabled={busy || loading}
        onClick={() => {setError(""); void reload().catch((err: unknown) => setError(messageOf(err)));}}>Обновить</button>
    </nav>

    {loading && !status && <p role="status">Подключение к AI-модулю…</p>}
    {error && <p className="ai-error" role="alert">{error}</p>}
    {notice && <p className="ai-notice" role="status">{notice}</p>}
    {gpuNotice.length > 0 && <div className="ai-notice" role="status">
      Новое оборудование: {gpuNotice.join(", ")}.
      <button type="button" className="button secondary" onClick={() => {setTab("local"); setGpuNotice([]);}}>
        Посмотреть доступные GPU
      </button>
    </div>}

    {status && <p className="ai-status">AI Agent {status.version} · Подключено моделей: {status.providers} ·
      Выполнение команд ядра: {status.core_actions_enabled ? "включено" : "отключено"}</p>}

    {/* Always mounted: GPU hot-plug events are detected while using chat too. */}
    <div hidden={tab !== "local"}>
      <AILocalPage
        onNewGPU={(names) => setGpuNotice(names)}
        onModelsReady={() => {void reload().catch(() => {});}}
        onOpenChat={() => {void reload().then(() => setTab("chat")).catch((err: unknown) => setError(messageOf(err)));}}
      />
    </div>

    {tab === "chat" && <section aria-label="Чат с AI">
      <Panel title="Разговор с агентом">
        <div className="ai-controls">
          <label>Модель
            <select className="text-input" value={modelId} onChange={(e) => setModelId(e.target.value)} disabled={busy}>
              {providers.length === 0 && <option value="">Сначала добавьте провайдера</option>}
              {providers.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.model}</option>)}
            </select>
          </label>
          <label>Диалог
            <select className="text-input" value={conversationId} onChange={(e) => setConversationId(e.target.value)} disabled={busy}>
              <option value="">Новый диалог</option>
              {conversations.map((item) => <option key={item.id} value={item.id}>{item.title}</option>)}
            </select>
          </label>
          <button type="button" className="button secondary" disabled={busy}
            onClick={() => {setConversationId(""); setMessages([]);}}>Новый диалог</button>
        </div>
        {!providers.length && <div className="ai-note">
          Провайдер не подключён. Добавьте Ollama или внешнюю модель на странице «AI-провайдеры».
          <button type="button" className="button secondary" onClick={() => setTab("providers")}>Настроить AI</button>
        </div>}
        <div className="ai-messages" aria-live="polite" aria-label="Сообщения">
          {messages.length === 0 && <p className="muted">История диалога появится здесь.</p>}
          {messages.map((item, index) => <article key={index} className={"ai-message " + (item.role === "user" ? "ai-human" : "ai-assistant")}>
            <strong>{item.role === "user" ? "Вы" : "AI"}</strong>
            <p>{item.content}</p>
          </article>)}
        </div>
        <form onSubmit={(event) => {void send(event);}} className="ai-compose">
          <label htmlFor="ai-message">Сообщение агенту</label>
          <textarea id="ai-message" className="text-input" rows={3} maxLength={6000} required
            value={draft} disabled={busy} placeholder="Напишите задачу…"
            onChange={(e) => setDraft(e.target.value)}/>
          <button className="button primary" disabled={busy || !modelId || !draft.trim()} type="submit">
            {busy ? "Агент отвечает…" : "Отправить"}
          </button>
        </form>
      </Panel>
    </section>}

    {tab === "providers" && <section aria-label="Настройки AI-провайдеров">
      <Panel title="Подключённые модели">
        {!providers.length && <p>Провайдеры пока не добавлены.</p>}
        <div className="ai-provider-list">
          {providers.map((provider) => <article key={provider.id} className="ai-provider">
            <div><strong>{provider.name}</strong><p>{provider.kind} · {provider.model}</p>
              <p className="muted">{provider.base_url}</p></div>
            <button type="button" className="button danger" disabled={busy}
              onClick={() => {if (window.confirm(`Удалить провайдера «${provider.name}»?`)) {
                void run(() => api.aiDeleteProvider(provider.id), "Провайдер удалён.");
              }}}>Удалить</button>
          </article>)}
        </div>
      </Panel>
      <Panel title="Добавить провайдера">
        <form className="ai-provider-form" onSubmit={(event) => {void addProvider(event);}}>
          <label>Название<input className="text-input" maxLength={80} required value={name}
            onChange={(e) => setName(e.target.value)} placeholder="Домашний AI"/></label>
          <label>Тип
            <select className="text-input" value={kind} onChange={(e) => {
              const value = e.target.value as AIProvider["kind"];
              setKind(value); setBaseURL(value === "openai" ? "https://api.openai.com/v1" : "");
            }}>
              <option value="ollama">Ollama (локальный)</option>
              <option value="openai">OpenAI</option>
              <option value="openai_compatible">OpenAI-compatible / llama.cpp / LM Studio</option>
            </select>
          </label>
          <label>URL API<input className="text-input" type="url" required value={baseURL}
            onChange={(e) => setBaseURL(e.target.value)}
            placeholder={kind === "ollama" ? "http://192.168.1.20:11434" : "https://api.openai.com/v1"} /></label>
          <label>Модель<input className="text-input" maxLength={120} required value={model}
            onChange={(e) => setModel(e.target.value)} placeholder="Например, qwen2.5:7b"/></label>
          <label>Ключ API (при необходимости)<input className="text-input" type="password" autoComplete="off"
            value={apiKey} onChange={(e) => setApiKey(e.target.value)} /></label>
          <p className="muted">Ollama должен быть доступен из Docker-контейнера AI.
            Адрес 127.0.0.1 внутри контейнера не указывает на домашний сервер.
            Ключ хранится в данных AI-модуля и не возвращается браузеру.</p>
          <button type="submit" disabled={busy} className="button primary">Сохранить провайдера</button>
        </form>
      </Panel>
    </section>}

    {tab === "approvals" && <section aria-label="Подтверждения действий AI">
      <div className="ai-note" role="status">Подтверждение сейчас только фиксирует решение.
        AI Agent не выполняет команды в Core, не удаляет файлы и не управляет устройствами.</div>
      <Panel title="Запросы агента">
        {!approvals.length && <p>Запросов пока нет.</p>}
        <div className="ai-provider-list">
          {approvals.map((item) => <article className="ai-provider" key={item.id}>
            <div><strong>{item.action} → {item.target}</strong><p>{item.reason}</p><small>{item.status}</small></div>
            {item.status === "pending" && <div className="button-row">
              <button type="button" className="button secondary" disabled={busy}
                onClick={() => {void run(() => api.aiDecide(item.id, "approve"), "Решение сохранено. Команда не выполнена.");}}>
                Разрешить (без выполнения)</button>
              <button type="button" className="button danger" disabled={busy}
                onClick={() => {void run(() => api.aiDecide(item.id, "reject"), "Запрос отклонён.");}}>Отклонить</button>
            </div>}
          </article>)}
        </div>
      </Panel>
    </section>}
  </div>;
}
