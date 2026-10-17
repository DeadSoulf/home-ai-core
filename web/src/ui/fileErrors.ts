import { APIError } from "../api/client";
import { errorText, uiText, type UILocale } from "./messages";

/**
 * A 5xx response is not proof that Core is offline. Preserve the API code and
 * request ID in the ErrorNotice while keeping the user-facing summary safe.
 */
export function describeFileError(reason: unknown, locale: UILocale, fallback: string): string {
  if (reason instanceof APIError) {
    // Missing/moved NAS mounts are not generic permission failures. Give
    // administrators the exact remediation while retaining request ID and
    // unmodified backend diagnostics in ErrorNotice.
    if ((reason.code === "file_folder_prepare_failed" || reason.code === "file_pool_prepare_failed") &&
        (reason.rawMessage.includes("must be the filesystem mount point itself") ||
         reason.rawMessage.includes("updater helper mount namespace"))) {
      return uiText("filePoolMountRepairHint", locale);
    }
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
