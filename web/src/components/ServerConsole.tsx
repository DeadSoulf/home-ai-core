import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { api } from "../api/client";
import { Panel } from "./Panel";
import { useI18n } from "../i18n";

type ConsoleStatus = "connecting" | "connected" | "disconnected" | "error";

type ConsoleMessage = {
  type: "ready" | "output" | "exit" | "error";
  data?: string;
  code?: number;
};

const maxOutputChars = 256 * 1024;

export function ServerConsole() {
  const {t} = useI18n();
  const socketRef = useRef<WebSocket>();
  const outputRef = useRef<HTMLPreElement>(null);
  const historyRef = useRef<string[]>([]);
  const historyIndexRef = useRef(0);
  const [status, setStatus] = useState<ConsoleStatus>("disconnected");
  const [output, setOutput] = useState("");
  const [command, setCommand] = useState("");

  const append = (value: string) => {
    setOutput((current) => (current + value).slice(-maxOutputChars));
  };

  const disconnect = () => {
    const socket = socketRef.current;
    socketRef.current = undefined;
    if (socket && socket.readyState < WebSocket.CLOSING) {
      socket.close(1000, "console closed");
    }
    setStatus("disconnected");
  };

  const connect = async () => {
    if (status === "connecting" || status === "connected") return;
    setStatus("connecting");
    try {
      const ticket = await api.consoleTicket();
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      const socket = new WebSocket(
        `${protocol}//${window.location.host}/api/v1/system/console?ticket=${encodeURIComponent(ticket.ticket)}`,
      );
      socketRef.current = socket;

      socket.onopen = () => setStatus("connected");
      socket.onmessage = (event) => {
        let message: ConsoleMessage;
        try {
          message = JSON.parse(String(event.data)) as ConsoleMessage;
        } catch {
          append(String(event.data));
          return;
        }
        if (message.type === "ready" || message.type === "output") {
          if (message.data) append(message.data);
        } else if (message.type === "error") {
          append(`\n[console] ${message.data || t("requestFailed")}\n`);
          setStatus("error");
        } else if (message.type === "exit") {
          append(`\n[console] ${t("consoleProcessExited").replace("{code}", String(message.code ?? 0))}\n`);
        }
      };
      socket.onerror = () => setStatus("error");
      socket.onclose = () => {
        socketRef.current = undefined;
        setStatus((current) => current === "error" ? current : "disconnected");
      };
    } catch (reason) {
      append(`\n[console] ${reason instanceof Error ? reason.message : t("requestFailed")}\n`);
      setStatus("error");
    }
  };

  useEffect(() => {
    void connect();
    return () => {
      const socket = socketRef.current;
      socketRef.current = undefined;
      socket?.close(1000, "console page closed");
    };
    // Connect once when the console tab is mounted.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const element = outputRef.current;
    if (element) element.scrollTop = element.scrollHeight;
  }, [output]);

  const send = (event?: FormEvent) => {
    event?.preventDefault();
    const value = command.trimEnd();
    const socket = socketRef.current;
    if (!value || !socket || socket.readyState !== WebSocket.OPEN) return;
    const history = historyRef.current;
    if (history[history.length - 1] !== value) history.push(value);
    historyIndexRef.current = history.length;
    socket.send(JSON.stringify({type: "input", data: value + "\n"}));
    setCommand("");
  };

  const commandKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      send();
      return;
    }
    if (event.key === "ArrowUp" && !event.shiftKey && command.indexOf("\n") === -1) {
      const history = historyRef.current;
      if (!history.length) return;
      event.preventDefault();
      historyIndexRef.current = Math.max(0, historyIndexRef.current - 1);
      setCommand(history[historyIndexRef.current] || "");
      return;
    }
    if (event.key === "ArrowDown" && !event.shiftKey && command.indexOf("\n") === -1) {
      const history = historyRef.current;
      if (!history.length) return;
      event.preventDefault();
      historyIndexRef.current = Math.min(history.length, historyIndexRef.current + 1);
      setCommand(history[historyIndexRef.current] || "");
    }
  };

  const statusLabel =
    status === "connected" ? t("consoleStatusConnected") :
    status === "connecting" ? t("consoleStatusConnecting") :
    status === "error" ? t("consoleStatusError") :
    t("consoleStatusDisconnected");

  return (
    <Panel
      className="wide console-panel"
      title={t("console")}
      action={
        <div className="console-actions">
          <span className={`status-badge ${status === "connected" ? "status-success" : status === "error" ? "status-failed" : ""}`}>
            {statusLabel}
          </span>
          <button type="button" className="button secondary" onClick={() => setOutput("")}>{t("consoleClear")}</button>
          {status === "connected"
            ? <button type="button" className="button secondary" onClick={disconnect}>{t("consoleDisconnect")}</button>
            : <button type="button" className="button primary" disabled={status === "connecting"} onClick={() => void connect()}>
                {status === "connecting" ? t("consoleConnecting") : t("consoleConnect")}
              </button>}
        </div>
      }
    >
      <div className="notice console-security-notice">{t("consoleSecurityNotice")}</div>
      <pre ref={outputRef} className="server-console-output" aria-live="polite">{output || t("consoleWaiting")}</pre>
      <form className="server-console-input" onSubmit={send}>
        <span className="mono">home-ai-core $</span>
        <textarea
          rows={2}
          value={command}
          disabled={status !== "connected"}
          placeholder={t("consoleCommandPlaceholder")}
          onChange={(event) => setCommand(event.target.value)}
          onKeyDown={commandKeyDown}
          spellCheck={false}
          autoCapitalize="none"
          autoCorrect="off"
        />
        <button type="submit" className="button primary" disabled={status !== "connected" || !command.trim()}>
          {t("consoleRun")}
        </button>
      </form>
      <div className="console-hint">{t("consoleHint")}</div>
    </Panel>
  );
}
