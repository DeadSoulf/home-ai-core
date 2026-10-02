import {FormEvent, useEffect, useMemo, useState} from "react";
import {api, APIError} from "../api/client";
import type {AIConversation, AIMessage, AIStatus} from "../api/types";
import {useI18n} from "../i18n";

export function AIPage() {
  const {t, date} = useI18n();
  const [status, setStatus] = useState<AIStatus>();
  const [conversations, setConversations] = useState<AIConversation[]>([]);
  const [activeID, setActiveID] = useState("");
  const [messages, setMessages] = useState<AIMessage[]>([]);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [closing, setClosing] = useState(false);
  const [error, setError] = useState("");

  const active = useMemo(
    () => conversations.find((item) => item.id === activeID),
    [conversations, activeID],
  );

  const loadMessages = async (id: string) => {
    if (!id) {
      setMessages([]);
      return;
    }
    try {
      setMessages(await api.aiMessages(id));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
    }
  };

  const refreshConversations = async (preferID?: string) => {
    const items = await api.aiConversations();
    setConversations(items);
    const next = preferID && items.some((item) => item.id === preferID)
      ? preferID
      : (activeID && items.some((item) => item.id === activeID) ? activeID : items[0]?.id || "");
    setActiveID(next);
    return next;
  };

  useEffect(() => {
    let stopped = false;
    (async () => {
      try {
        const [aiStatus, items] = await Promise.all([api.aiStatus(), api.aiConversations()]);
        if (stopped) return;
        setStatus(aiStatus);
        setConversations(items);
        const first = items[0]?.id || "";
        setActiveID(first);
        if (first) setMessages(await api.aiMessages(first));
      } catch (reason) {
        if (!stopped) setError(reason instanceof Error ? reason.message : t("requestFailed"));
      } finally {
        if (!stopped) setLoading(false);
      }
    })();
    return () => { stopped = true; };
  }, [t]);

  const createConversation = async () => {
    setError("");
    try {
      const created = await api.createAIConversation();
      setConversations((items) => [created, ...items]);
      setActiveID(created.id);
      setMessages([]);
      return created.id;
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
      return "";
    }
  };

  const finishConversation = async () => {
    if (!activeID || !active || active.closed_at || closing || sending) return;
    if (!window.confirm(t("aiFinishConversationConfirm"))) return;

    setClosing(true);
    setError("");
    try {
      const closed = await api.closeAIConversation(activeID);
      setConversations((items) => items.map((item) => item.id === closed.id ? closed : item));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setClosing(false);
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const content = draft.trim();
    if (!content || sending || active?.closed_at) return;
    setSending(true);
    setError("");
    try {
      let id = activeID;
      if (!id) id = await createConversation();
      if (!id) return;

      setDraft("");
      const localUser: AIMessage = {
        id: "pending-" + Date.now(),
        conversation_id: id,
        role: "user",
        content,
        created_at: new Date().toISOString(),
      };
      setMessages((items) => [...items, localUser]);

      const result = await api.sendAIMessage(id, content);
      setMessages((items) => [
        ...items.filter((item) => item.id !== localUser.id),
        result.user_message,
        result.assistant_message,
      ]);
      await refreshConversations(id);
    } catch (reason) {
      if (reason instanceof APIError && reason.code === "ai_chat_unavailable") {
        setError(t("aiProviderUnavailable"));
      } else {
        setError(reason instanceof Error ? reason.message : t("requestFailed"));
      }
      if (activeID) await loadMessages(activeID);
    } finally {
      setSending(false);
    }
  };

  if (loading) {
    return <section className="page"><div className="panel">{t("loading")}</div></section>;
  }

  return (
    <section className="page ai-page">
      <div className="page-heading">
        <div>
          <h1>{t("aiAgent")}</h1>
          <p>{t("aiAgentSubtitle")}</p>
        </div>
        <div className="ai-provider-state">
          <span className={status?.provider_configured ? "badge ok" : "badge warning"}>
            {status?.provider_configured ? t("aiProviderReady") : t("aiProviderMissing")}
          </span>
          {status?.provider_configured && (
            <small>{status.provider_id}{status.provider_model ? " · " + status.provider_model : ""}</small>
          )}
        </div>
      </div>

      {!status?.provider_configured && (
        <div className="notice warning">
          <strong>{t("aiProviderMissing")}</strong>
          <p>{t("aiProviderSetupHint")}</p>
        </div>
      )}

      {error && <div className="form-error">{error}</div>}

      <div className="ai-layout">
        <aside className="panel ai-conversations">
          <div className="panel-header">
            <div>
              <h2>{t("aiConversations")}</h2>
              <p>{t("aiConversationsHint")}</p>
            </div>
            <button type="button" className="button primary compact" onClick={() => void createConversation()}>
              {t("aiNewConversation")}
            </button>
          </div>
          <div className="ai-conversation-list">
            {conversations.length === 0 && <p className="muted">{t("aiNoConversations")}</p>}
            {conversations.map((item) => (
              <button
                type="button"
                key={item.id}
                className={item.id === activeID ? "ai-conversation active" : "ai-conversation"}
                onClick={() => {
                  setActiveID(item.id);
                  setError("");
                  void loadMessages(item.id);
                }}
              >
                <strong>{item.title || t("aiUntitledConversation")}</strong>
                <span>
                  {item.closed_at ? t("aiConversationFinished") + " · " : ""}
                  {date(item.updated_at)}
                </span>
              </button>
            ))}
          </div>
        </aside>

        <div className="panel ai-chat">
          <div className="ai-chat-head">
            <div>
              <div className="ai-chat-title-row">
                <h2>{active?.title || t("aiNewConversation")}</h2>
                {active?.closed_at && <span className="badge">{t("aiConversationFinished")}</span>}
              </div>
              <p>{active?.closed_at ? t("aiFinishedConversationHint") : t("aiLocalOnlyNotice")}</p>
            </div>
            <div className="ai-chat-head-actions">
              {active && !active.closed_at && (
                <button
                  type="button"
                  className="button secondary compact"
                  disabled={sending || closing}
                  onClick={() => void finishConversation()}
                >
                  {closing ? t("working") : t("aiFinishConversation")}
                </button>
              )}
              {active?.closed_at && (
                <button
                  type="button"
                  className="button primary compact"
                  onClick={() => void createConversation()}
                >
                  {t("aiNewConversation")}
                </button>
              )}
            </div>
          </div>

          <div className="ai-message-list" aria-live="polite">
            {messages.length === 0 && (
              <div className="ai-empty-state">
                <strong>{t("aiStartConversation")}</strong>
                <span>{t("aiStartConversationHint")}</span>
              </div>
            )}
            {messages.map((message) => (
              <article key={message.id} className={"ai-message " + message.role}>
                <div className="ai-message-meta">
                  <strong>{message.role === "user" ? t("you") : t("aiAgent")}</strong>
                  <span>{date(message.created_at)}</span>
                </div>
                <p>{message.content}</p>
              </article>
            ))}
            {sending && <div className="ai-thinking">{t("aiThinking")}</div>}
          </div>

          <form className="ai-composer" onSubmit={submit}>
            <textarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              placeholder={active?.closed_at ? t("aiFinishedConversationPlaceholder") : t("aiMessagePlaceholder")}
              maxLength={8000}
              rows={3}
              disabled={sending || Boolean(active?.closed_at) || !status?.provider_configured}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  event.currentTarget.form?.requestSubmit();
                }
              }}
            />
            <div className="ai-composer-actions">
              <span>{t("aiEnterHint")}</span>
              <button
                type="submit"
                className="button primary"
                disabled={sending || Boolean(active?.closed_at) || !draft.trim() || !status?.provider_configured}
              >
                {sending ? t("aiSending") : t("aiSend")}
              </button>
            </div>
          </form>
        </div>
      </div>
    </section>
  );
}
