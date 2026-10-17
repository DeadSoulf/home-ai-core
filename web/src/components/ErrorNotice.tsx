import { useI18n } from "../i18n";
import { errorText } from "../ui/messages";
import { useUI } from "../ui/useUI";
export function ErrorNotice({error, message, id}: {error?: unknown; message?: string; id?: string}) {
  const {locale} = useI18n();
  const u = useUI();
  const detail = error as {rawMessage?: string; message?: string; code?: string; requestId?: string; status?: number} | undefined;
  return <div className="error-notice" id={id}>
    <p className="form-error" role="alert">{message || errorText(error, locale)}</p>
    {detail && <details className="technical-details"><summary>{u("details")}</summary>
      <p className="mono">{detail.status ? `HTTP ${detail.status} · ` : ""}{detail.code}</p><p>{detail.rawMessage || detail.message}</p>
      {detail.requestId && <p className="mono">request_id: {detail.requestId}</p>}
    </details>}
  </div>;
}
