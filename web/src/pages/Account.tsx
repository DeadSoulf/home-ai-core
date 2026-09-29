import type { Actor } from "../api/types";
import { Panel } from "../components/Panel";
import { useI18n } from "../i18n";
import { PageHeading } from "./Dashboard";

export function AccountPage({actor}: {actor: Actor}) {
  const {t} = useI18n();
  return (
    <div className="page">
      <PageHeading title={t("home")} subtitle={t("limitedHomeSubtitle")} />
      <Panel title={t("yourAccount")}>
        <dl className="details">
          <dt>{t("displayName")}</dt>
          <dd>{actor.display_name || "—"}</dd>
          <dt>{t("username")}</dt>
          <dd className="mono">{actor.username || "—"}</dd>
          <dt>{t("roles")}</dt>
          <dd>{actor.roles.join(", ") || "—"}</dd>
        </dl>
        <p className="muted account-access-note">{t("limitedAccessNotice")}</p>
      </Panel>
    </div>
  );
}
