import {FormEvent, useEffect, useMemo, useState} from "react";
import {api, APIError} from "../api/client";
import type {AIAction, AIConversation, AIMessage, AIStatus} from "../api/types";
import {useI18n} from "../i18n";

export function AIPage() {
  const {t, date} = useI18n();
  const [status, setStatus] = useState<AIStatus>();
  const [conversations, setConversations] = useState<AIConversation[]>([]);
  const [activeID, setActiveID] = useState("");
  const [messages, setMessages] = useState<AIMessage[]>([]);
  const [actions, setActions] = useState<AIAction[]>([]);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [closing, setClosing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [actionBusy, setActionBusy] = useState("");
  const [error, setError] = useState("");

  const actionStatusLabel = (value: AIAction["status"]) => {
    switch (value) {
      case "pending": return t("aiActionStatusPending");
      case "executing": return t("aiActionStatusExecuting");
      case "executed": return t("aiActionStatusExecuted");
      case "rejected": return t("aiActionStatusRejected");
      case "failed": return t("aiActionStatusFailed");
    }
  };

  const active = useMemo(
    () => conversations.find((item) => item.id === activeID),
    [conversations, activeID],
  );
  const closedCount = useMemo(
    () => conversations.filter((item) => Boolean(item.closed_at)).length,
    [conversations],
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

  const loadActions = async (id: string) => {
    if (!id) {
      setActions([]);
      return;
    }
    try {
      setActions(await api.aiActions(id));
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
        if (first) {
          const [firstMessages, firstActions] = await Promise.all([
            api.aiMessages(first),
            api.aiActions(first),
          ]);
          setMessages(firstMessages);
          setActions(firstActions);
        }
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
      setActions([]);
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
      // A connection can be interrupted after Core has already committed closed_at.
      // Reconcile from authoritative server state before surfacing a network error.
      try {
        const items = await api.aiConversations();
        setConversations(items);
        if (items.some((item) => item.id === activeID && Boolean(item.closed_at))) {
          return;
        }
      } catch {
        // Preserve the original close error when reconciliation is also unavailable.
      }
      setError(reason instanceof APIError ? reason.message : t("requestFailed"));
    } finally {
      setClosing(false);
    }
  };

  const deleteConversation = async () => {
    if (!activeID || !active?.closed_at || deleting || clearing) return;
    if (!window.confirm(t("aiDeleteConversationConfirm"))) return;

    setDeleting(true);
    setError("");
    try {
      await api.deleteAIConversation(activeID);
      const next = await refreshConversations();
      await Promise.all([loadMessages(next), loadActions(next)]);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setDeleting(false);
    }
  };

  const clearFinishedConversations = async () => {
    if (closedCount === 0 || deleting || clearing) return;
    if (!window.confirm(t("aiClearFinishedConfirm"))) return;

    setClearing(true);
    setError("");
    try {
      await api.clearClosedAIConversations();
      const preferred = active && !active.closed_at ? active.id : undefined;
      const next = await refreshConversations(preferred);
      await Promise.all([loadMessages(next), loadActions(next)]);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setClearing(false);
    }
  };

  const decideAction = async (action: AIAction, decision: "approve" | "reject") => {
    if (!activeID || actionBusy || active?.closed_at) return;
    setActionBusy(action.id);
    setError("");
    try {
      const updated = decision === "approve"
        ? await api.approveAIAction(activeID, action.id)
        : await api.rejectAIAction(activeID, action.id);
      setActions((items) => items.map((item) => item.id === updated.id ? updated : item));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("requestFailed"));
      await loadActions(activeID);
    } finally {
      setActionBusy("");
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
      const timestamp = Date.now();
      const localUser: AIMessage = {
        id: "pending-user-" + timestamp,
        conversation_id: id,
        role: "user",
        content,
        created_at: new Date().toISOString(),
      };
      const localAssistant: AIMessage = {
        id: "streaming-assistant-" + timestamp,
        conversation_id: id,
        role: "assistant",
        content: "",
        created_at: new Date().toISOString(),
      };
      setMessages((items) => [...items, localUser, localAssistant]);

      const result = await api.streamAIMessage(id, content, (delta) => {
        setMessages((items) => items.map((item) =>
          item.id === localAssistant.id
            ? {...item, content: item.content + delta}
            : item,
        ));
      });
      setMessages((items) => [
        ...items.filter((item) => item.id !== localUser.id && item.id !== localAssistant.id),
        result.user_message,
        result.assistant_message,
      ]);
      await Promise.all([refreshConversations(id), loadActions(id)]);
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
            <div className="ai-conversation-header-actions">
              {closedCount > 0 && (
                <button
                  type="button"
                  className="button secondary compact"
                  disabled={clearing || deleting || sending || closing}
                  onClick={() => void clearFinishedConversations()}
                >
                  {clearing ? t("working") : t("aiClearFinished")}
                </button>
              )}
              <button
                type="button"
                className="button primary compact"
                disabled={clearing || deleting}
                onClick={() => void createConversation()}
              >
                {t("aiNewConversation")}
              </button>
            </div>
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
                  void Promise.all([loadMessages(item.id), loadActions(item.id)]);
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
              <button
                type="button"
                className="button secondary compact"
                disabled={!active || Boolean(active.closed_at) || sending || closing}
                onClick={() => void finishConversation()}
              >
                {closing ? t("working") : t("aiFinishConversation")}
              </button>
              {active?.closed_at && (
                <>
                  <button
                    type="button"
                    className="button danger compact"
                    disabled={deleting || clearing}
                    onClick={() => void deleteConversation()}
                  >
                    {deleting ? t("working") : t("aiDeleteConversation")}
                  </button>
                  <button
                    type="button"
                    className="button primary compact"
                    disabled={deleting || clearing}
                    onClick={() => void createConversation()}
                  >
                    {t("aiNewConversation")}
                  </button>
                </>
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
            {actions.map((action) => (
              <article key={action.id} className={"ai-action-card " + action.status}>
                <div className="ai-action-head">
                  <div>
                    <strong>{action.tool_name}</strong>
                    <span>{action.tool_id}</span>
                  </div>
                  <span className={action.sensitivity === "sensitive" ? "badge warning" : "badge"}>
                    {action.sensitivity === "sensitive" ? t("aiActionSensitive") : t("aiActionChange")}
                  </span>
                </div>
                <pre>{safeActionJSON(action.input)}</pre>
                <div className="ai-action-footer">
                  <span>{t("aiActionStatus")}: {actionStatusLabel(action.status)}</span>
                  {action.status === "pending" && !active?.closed_at && (
                    <div className="ai-action-buttons">
                      <button
                        type="button"
                        className="button secondary compact"
                        disabled={Boolean(actionBusy)}
                        onClick={() => void decideAction(action, "reject")}
                      >
                        {t("aiActionReject")}
                      </button>
                      <button
                        type="button"
                        className="button primary compact"
                        disabled={Boolean(actionBusy)}
                        onClick={() => void decideAction(action, "approve")}
                      >
                        {actionBusy === action.id ? t("working") : t("aiActionApprove")}
                      </button>
                    </div>
                  )}
                </div>
                {action.status === "executed" && action.result !== undefined && (
                  <details className="ai-action-result">
                    <summary>{t("aiActionResult")}</summary>
                    <pre>{safeActionJSON(action.result)}</pre>
                  </details>
                )}
                {action.status === "failed" && (
                  <div className="form-error">{t("aiActionFailed")}: {action.error_code || t("unknown")}</div>
                )}
              </article>
            ))}
            {sending && !messages.some((message) => message.id.startsWith("streaming-assistant-") && message.content) && (
              <div className="ai-thinking">{t("aiThinking")}</div>
            )}
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


function safeActionJSON(value: unknown): string {
  try {
    return JSON.stringify(redactActionValue(value), null, 2);
  } catch {
    return "{}";
  }
}

function redactActionValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(redactActionValue);
  }
  if (value && typeof value === "object") {
    const result: Record<string, unknown> = {};
    for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
      if (/password|secret|token|private|preshared/i.test(key)) {
        result[key] = "••••••";
      } else {
        result[key] = redactActionValue(item);
      }
    }
    return result;
  }
  return value;
}
