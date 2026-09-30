import { useCallback } from "react";
import { api } from "../api/client";
import type { NetworkProfile, SystemReadiness } from "../api/types";
import { useI18n } from "../i18n";
import { useResource } from "../hooks/useResource";
import { ErrorState, LoadingState, Panel } from "./Panel";
import { Status } from "../pages/Dashboard";

export function ReadinessDiagnostics({revision}: {revision: number}) {
  const {t, date} = useI18n();
  const load = useCallback(() => api.systemReadiness(), []);
  const resource = useResource(load, revision);

  if (resource.loading && !resource.data) return <LoadingState />;
  if (resource.error && !resource.data) return <ErrorState message={resource.error} />;

  const readiness = resource.data;
  if (!readiness) return null;

  return (
    <Panel title={t("readinessTitle")} className="wide">
      <div className="readiness-header">
        <span className="muted">{t("readinessSubtitle")}</span>
        <span className="muted small">{t("readinessChecked")}: {date(readiness.checked_at)}</span>
      </div>

      <div className="readiness-grid">
        <ReadinessNetwork readiness={readiness} />
        <ReadinessWireGuard readiness={readiness} />
        <ReadinessUpdater readiness={readiness} />
      </div>
    </Panel>
  );
}

function ReadinessNetwork({readiness}: {readiness: SystemReadiness}) {
  const {t} = useI18n();
  const network = readiness.network;
  if (!network) {
    return <ReadinessCard title={t("readinessNetwork")} state="unavailable" details={t("readinessPermissionUnavailable")} />;
  }

  const interfaces = network.interfaces.filter((iface) => !iface.loopback);
  const profiles = new Map(network.profiles.map((profile) => [profile.interface, profile]));
  const healthy = !network.error && interfaces.length > 0;

  return (
    <ReadinessCard
      title={t("readinessNetwork")}
      state={healthy ? "ready" : "warning"}
      details={network.error || `${t("networkBackend")}: ${network.backend || "unknown"}`}
    >
      <div className="table-wrap readiness-table">
        <table>
          <thead>
            <tr>
              <th>{t("name")}</th>
              <th>{t("networkCurrentState")}</th>
              <th>{t("networkCurrentIpv4")}</th>
              <th>{t("networkProfileMethod")}</th>
              <th>{t("networkProfileOwnership")}</th>
            </tr>
          </thead>
          <tbody>
            {interfaces.map((iface) => {
              const profile = profiles.get(iface.name);
              return (
                <tr key={iface.name}>
                  <td className="mono">{iface.name}</td>
                  <td><Status value={iface.oper_state || (iface.up ? "up" : "down")} /></td>
                  <td className="mono">{currentIPv4(iface.addresses).join(", ") || "—"}</td>
                  <td>{profile?.method || "—"}</td>
                  <td>{profileOwnership(profile, t)}</td>
                </tr>
              );
            })}
            {interfaces.length === 0 && (
              <tr><td colSpan={5} className="muted">{t("readinessNoInterfaces")}</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </ReadinessCard>
  );
}

function ReadinessWireGuard({readiness}: {readiness: SystemReadiness}) {
  const {t} = useI18n();
  const wireguard = readiness.wireguard;
  if (!wireguard) {
    return <ReadinessCard title="WireGuard" state="unavailable" details={t("readinessPermissionUnavailable")} />;
  }
  if (wireguard.error) {
    return <ReadinessCard title="WireGuard" state="warning" details={wireguard.error} />;
  }
  if (!wireguard.available) {
    return <ReadinessCard title="WireGuard" state="warning" details={t("wireGuardUnavailable")} />;
  }

  const tunnels = wireguard.tunnels || [];
  const ready = tunnels.every((tunnel) => !tunnel.active || tunnel.enabled);
  return (
    <ReadinessCard
      title="WireGuard"
      state={ready ? "ready" : "warning"}
      details={tunnels.length ? t("readinessWireGuardConfigured") : t("wireGuardNoTunnels")}
    >
      {tunnels.map((tunnel) => (
        <div className="readiness-row" key={tunnel.name}>
          <span className="mono">{tunnel.name}</span>
          <span>{tunnel.active ? t("active") : t("disabled")}</span>
          <span>{tunnel.enabled ? t("readinessAutostartEnabled") : t("readinessAutostartDisabled")}</span>
        </div>
      ))}
    </ReadinessCard>
  );
}

function ReadinessUpdater({readiness}: {readiness: SystemReadiness}) {
  const {t} = useI18n();
  const updater = readiness.updater;
  if (!updater) {
    return <ReadinessCard title={t("readinessUpdater")} state="unavailable" details={t("readinessPermissionUnavailable")} />;
  }

  const helperReady = updater.helper_available && updater.helper_compatible;
  return (
    <ReadinessCard
      title={t("readinessUpdater")}
      state={helperReady && updater.rollback_available ? "ready" : "warning"}
      details={updater.helper_error || `${t("coreVersion")}: ${updater.current_version}`}
    >
      <dl className="details readiness-details">
        <dt>{t("readinessHelper")}</dt>
        <dd>{helperReady ? t("readinessReady") : t("readinessNotReady")}</dd>
        <dt>{t("rollbackAvailable")}</dt>
        <dd>
          {updater.rollback_available
            ? `${t("yes")} · ${updater.rollback_version || "—"}`
            : t("no")}
        </dd>
      </dl>
    </ReadinessCard>
  );
}

function ReadinessCard({
  title,
  state,
  details,
  children,
}: {
  title: string;
  state: "ready" | "warning" | "unavailable";
  details: string;
  children?: React.ReactNode;
}) {
  const {t} = useI18n();
  const label = state === "ready"
    ? t("readinessReady")
    : state === "warning"
      ? t("readinessAttention")
      : t("readinessUnavailable");

  return (
    <section className="readiness-card">
      <div className="readiness-card-head">
        <strong>{title}</strong>
        <span className={`readiness-badge ${state}`}>{label}</span>
      </div>
      <p className="muted small">{details}</p>
      {children}
    </section>
  );
}

function currentIPv4(addresses: string[]): string[] {
  return addresses.filter((address) => {
    const host = address.split("/", 1)[0];
    return host !== "" && !host.includes(":");
  });
}

function profileOwnership(
  profile: NetworkProfile | undefined,
  t: (key: string) => string,
): string {
  switch (profile?.ownership) {
  case "home-ai":
    return t("networkProfileOwnedHomeAI");
  case "external":
    return t("networkProfileExternal");
  case "conflict":
    return t("networkProfileConflict");
  default:
    return profile?.managed ? t("yes") : t("networkProfileOwnershipNone");
  }
}
