import { useEffect, useRef, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { AIModel, AISession } from "../api/types";
import { useI18n } from "../i18n";

export function AIPage() {
  const {t, date} = useI18n();
  const [models, setModels] = useState<AIModel[]>([]);
  const [provider, setProvider] = useState("");
  const [sessions, setSessions] = useState<AISession[]>([]);
  const [selectedModel, setSelectedModel] = useState("");
  const [selectedID, setSelectedID] = useState("");
  const [active, setActive] = useState<AISession>();
  const [input, setInput] = useState("");
  const [pendingUser, setPendingUser] = useState("");
  const [streamed, setStreamed] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const abortRef = useRef<AbortController>();

  const refreshSessions = async () => {
    const items = await api.aiSessions();
    setSessions(items);
    return items;
  };

  useEffect(() => {
    let stopped = false;
    (async () => {
      setLoading(true);
      setError("");
      try {
        const [modelResult, sessionResult] = await Promise.allSettled([
          api.aiModels(),
          api.aiSessions(),
        ]);
        if (stopped) return;
        if (modelResult.status === "fulfilled") {
          setProvider(modelResult.value.provider);
          setModels(modelResult.value.models);
          setSelectedModel((current) => current || modelResult.value.models[0]?.id || "");
        } else {
          setError(t("aiProviderUnavailable"));
        }
        if (sessionResult.status === "fulfilled") {
          setSessions(sessionResult.value);
          if (sessionResult.value[0]) setSelectedID(sessionResult.value[0].id);
        }
      } finally {
        if (!stopped) setLoading(false);
      }
    })();
    return () => {
      stopped = true;
      abortRef.current?.abort();
    };
  }, [t]);

  useEffect(() => {
    if (!selectedID) {
      setActive(undefined);
      return;
    }
    let stopped = false;
    api.aiSession(selectedID)
      .then((item) => { if (!stopped) setActive(item); })
      .catch(() => { if (!stopped) setError(t("loadError")); });
    return () => { stopped = true; };
  }, [selectedID, t]);

  const createConversation = async () => {
    if (!selectedModel) return undefined;
    setError("");
    try {
      const item = await api.createAISession(selectedModel);
      setSessions((current) => [item, ...current]);
      setSelectedID(item.id);
      setActive(item);
      return item;
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
      return undefined;
    }
  };

  const send = async (event: FormEvent) => {
    event.preventDefault();
    const text = input.trim();
    if (!text || sending) return;

    let conversation = active;
    if (!conversation) conversation = await createConversation();
    if (!conversation) return;

    setInput("");
    setPendingUser(text);
    setStreamed("");
    setSending(true);
    setError("");
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const completed = await api.streamAIMessage(
        conversation.id,
        text,
        (delta) => setStreamed((current) => current + delta),
        controller.signal,
      );
      setActive(completed);
      setPendingUser("");
      setStreamed("");
      await refreshSessions();
    } catch (reason) {
      if (controller.signal.aborted) {
        setError(t("aiGenerationStopped"));
      } else {
        setError(reason instanceof Error ? reason.message : t("requestFailed"));
      }
      try {
        const current = await api.aiSession(conversation.id);
        setActive(current);
        await refreshSessions();
      } catch {
        // Keep the visible partial response if the refresh also fails.
      }
    } finally {
      abortRef.current = undefined;
      setSending(false);
    }
  };

  const messages = active?.messages || [];

  return (
    <div className="page ai-page">
      <div className="page-heading ai-heading">
        <div>
          <h1>{t("aiAgent")}</h1>
          <p>{t("aiSubtitle")}</p>
        </div>
        <div className="ai-provider-badge">
          <span className={models.length ? "status-dot connected" : "status-dot disconnected"} />
          <span>{provider || "Ollama"}</span>
        </div>
      </div>

      {error && <div className="notice danger">{error}</div>}

      <div className="ai-layout">
        <aside className="panel ai-session-panel">
          <div className="panel-header">
            <h2>{t("aiSessions")}</h2>
          </div>
          <label className="field">
            <span>{t("aiModel")}</span>
            <select value={selectedModel} onChange={(event) => setSelectedModel(event.target.value)} disabled={!models.length || sending}>
              {!models.length && <option value="">{t("aiNoModels")}</option>}
              {models.map((model) => <option key={model.id} value={model.id}>{model.name || model.id}</option>)}
            </select>
          </label>
          <button type="button" className="button primary full" disabled={!selectedModel || sending} onClick={() => void createConversation()}>
            {t("aiNewChat")}
          </button>
          {!models.length && !loading && <p className="muted small">{t("aiOllamaHint")}</p>}
          <div className="ai-session-list">
            {sessions.map((session) => (
              <button
                type="button"
                key={session.id}
                className={selectedID === session.id ? "ai-session active" : "ai-session"}
                onClick={() => setSelectedID(session.id)}
                disabled={sending}
              >
                <strong>{session.title || t("aiConversation")}</strong>
                <span>{session.model}</span>
                <small>{date(session.updated_at)}</small>
              </button>
            ))}
            {!sessions.length && !loading && <p className="muted">{t("aiNoSessions")}</p>}
          </div>
        </aside>

        <section className="panel ai-chat-panel">
          {!active ? (
            <div className="ai-empty">
              <div className="brand-mark">AI</div>
              <h2>{models.length ? t("aiSelectConversation") : t("aiProviderUnavailable")}</h2>
              <p className="muted">{models.length ? t("aiCreateChat") : t("aiOllamaHint")}</p>
            </div>
          ) : (
            <>
              <div className="ai-chat-header">
                <div>
                  <h2>{active.title || t("aiConversation")}</h2>
                  <span className="muted small">{active.model} · {active.provider}</span>
                </div>
              </div>
              <div className="ai-messages" aria-live="polite">
                {messages.map((message) => (
                  <article key={message.id} className={"ai-message " + message.role}>
                    <span>{message.role === "user" ? t("user") : t("aiAgent")}</span>
                    <p>{message.content}</p>
                  </article>
                ))}
                {pendingUser && (
                  <article className="ai-message user pending">
                    <span>{t("user")}</span>
                    <p>{pendingUser}</p>
                  </article>
                )}
                {sending && (
                  <article className="ai-message assistant pending">
                    <span>{t("aiAgent")}</span>
                    <p>{streamed || t("aiThinking")}</p>
                  </article>
                )}
              </div>
              <form className="ai-composer" onSubmit={send}>
                <textarea
                  value={input}
                  onChange={(event) => setInput(event.target.value)}
                  placeholder={t("aiMessagePlaceholder")}
                  maxLength={16384}
                  rows={3}
                  disabled={sending}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" && !event.shiftKey) {
                      event.preventDefault();
                      event.currentTarget.form?.requestSubmit();
                    }
                  }}
                />
                <div className="ai-composer-actions">
                  {sending && (
                    <button type="button" className="button secondary" onClick={() => abortRef.current?.abort()}>
                      {t("aiStop")}
                    </button>
                  )}
                  <button type="submit" className="button primary" disabled={sending || !input.trim()}>
                    {t("aiSend")}
                  </button>
                </div>
              </form>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
