import {useI18n} from "../i18n";
import type {StoragePurpose} from "../api/types";

// This list is intentionally limited to purposes supported by Core's
// storage_purposes schema and reservation rules. Do not expose arbitrary
// installed modules until they declare a verified storage contract.
export const supportedStoragePurposes: readonly StoragePurpose[] = ["files", "nvr"];

export function StoragePurposeOptions({includeNone = false}: {includeNone?: boolean}) {
  const {t} = useI18n();
  return <>
    {includeNone && <option value="">{t("storagePurposeNone")}</option>}
    <option value="files">{t("storagePurposeFiles")}</option>
    <option value="nvr">{t("storagePurposeNVR")}</option>
  </>;
}
