import { APIError } from "../api/client";
import { errorText, uiText, type UILocale } from "./messages";

/**
 * A 5xx response is not proof that Core is offline. Preserve the API code and
 * request ID in the ErrorNotice while keeping the user-facing summary safe.
 */
export function describeFileError(reason: unknown, locale: UILocale, fallback: string): string {
  if (reason instanceof APIError) {
    const summary = errorText(reason, locale);
    if (reason.status >= 500 && summary === uiText("unavailable", locale)) {
      return uiText("fileApiResponseFailed", locale).replace("{status}", String(reason.status));
    }
    return summary;
  }
  if (reason instanceof TypeError || (reason instanceof Error &&
      (reason.name === "TimeoutError" || reason.name === "AbortError"))) {
    return errorText(reason, locale);
  }
  return reason instanceof Error ? reason.message || fallback : fallback;
}
