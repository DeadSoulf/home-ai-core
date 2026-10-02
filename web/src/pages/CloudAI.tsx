import {useEffect, useState} from "react";
import {api} from "../api/client";
import type {AIStatus} from "../api/types";
import {useI18n} from "../i18n";

export function CloudAIPage({onOpenAgent}: {onOpenAgent: () => void}) {
  const {t} = useI18n();
  const [status, setStatus] = useState<AIStatus>();
  const [error, setError] = useState("");

  useEffect(() => {
    let stopped = false;
    api.aiStatus()
      .then((value) => { if (!stopped) setStatus(value); })
      .catch((reason) => { if (!stopped) setError(reason instanceof Error ? reason.message : t("requestFailed")); });
    return () => { stopped = true; };
  }, [t]);

  return (
    <section className="page">
      <div className="page-heading">
        <div>
          <h1>{t("cloudAI")}</h1>
          <p>{t("cloudAISubtitle")}</p>
        </div>
        <span className={status?.cloud_provider_enabled ? "badge ok" : "badge warning"}>
          {status?.cloud_provider_enabled ? t("cloudAIEnabled") : t("cloudAIDisabled")}
        </span>
      </div>
      {error && <div className="form-error">{error}</div>}
      <div className="panel">
        <h2>{t("cloudAIProvider")}</h2>
        <p>
          {status?.cloud_provider_configured
            ? t("cloudAIConfigured")
            : t("cloudAINotConfigured")}
        </p>
        {status?.cloud_provider_model && (
          <dl className="details">
            <dt>{t("model")}</dt><dd className="mono">{status.cloud_provider_model}</dd>
          </dl>
        )}
        <div className="notice">
          <p>{t("cloudAISetupHint")}</p>
          <code>HOME_AI_CLOUD_AI_ENDPOINT</code><br />
          <code>HOME_AI_CLOUD_AI_MODEL</code><br />
          <code>HOME_AI_CLOUD_AI_API_KEY</code>
        </div>
        <div className="notice warning">
          <p>{t("cloudAIPrivacyHint")}</p>
        </div>
        <button type="button" className="button primary" onClick={onOpenAgent}>
          {t("cloudAIOpenAgent")}
        </button>
      </div>
    </section>
  );
}
