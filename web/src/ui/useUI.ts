import { useCallback } from "react";
import { useI18n } from "../i18n";
import { uiText, type UIKey } from "./messages";
export function useUI() {
  const {locale} = useI18n();
  return useCallback((key: UIKey) => uiText(key, locale), [locale]);
}
