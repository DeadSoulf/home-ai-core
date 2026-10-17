import type { Actor } from "../api/types";
import { PasswordChange } from "../components/PasswordChange";
import { Panel } from "../components/Panel";
import { SectionGuide } from "../components/SectionGuide";
import { useUI } from "../ui/useUI";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";

export function AccountPage({actor, onPasswordChanged}: {actor: Actor; onPasswordChanged: () => void}) {
  const {t} = useI18n();
  const u = useUI();
  return (
    <div className="page">
      <PageHeading title={t("yourAccount")} subtitle={t("limitedHomeSubtitle")} />
      <SectionGuide title={u("uxAccountGuide")} description={u("uxAccountGuideDescription")} />
      <Panel title={t("yourAccount")}>
        <dl className="details">
          <dt>{t("displayName")}</dt>
          <dd>{actor.display_name || "—"}</dd>
          <dt>{t("username")}</dt>
          <dd className="mono">{actor.username || "—"}</dd>
          <dt>{t("roles")}</dt>
          <dd>{actor.roles.map((role) => role === "administrator" || role === "owner" ? t("userProfileAdministrator") : role).join(", ") || "—"}</dd>
        </dl>
      </Panel>
      <Panel title={t("changePassword")}><PasswordChange onChanged={onPasswordChanged}/></Panel>
    </div>
  );
}
