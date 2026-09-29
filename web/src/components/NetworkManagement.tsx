import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { SystemResponse, WireGuardStatus, WireGuardTunnel } from "../api/types";
import { Panel } from "./Panel";
import { useI18n } from "../i18n";
import { Status } from "../pages/Dashboard";

type NetworkInterface = SystemResponse["system"]["network_interfaces"][number];

export function NetworkManagement(props: {
  interfaces: NetworkInterface[];
  canReadWireGuard: boolean;
  canManage: boolean;
  onChanged: () => void;
}) {
  const {t, date} = useI18n();
  const [wireGuard, setWireGuard] = useState<WireGuardStatus>();
  const [wireGuardError, setWireGuardError] = useState("");
  const [busy, setBusy] = useState("");
  const [message, setMessage] = useState("");
  const [operationError, setOperationError] = useState("");
  const [tunnelName, setTunnelName] = useState("wg0");
  const [tunnelAddress, setTunnelAddress] = useState("10.77.0.1/24");
  const [listenPort, setListenPort] = useState("51820");

  const refreshWireGuard = useCallback(async () => {
    if (!props.canReadWireGuard) return;
    try {
      const status = await api.wireGuardStatus();
      setWireGuard(status);
      setWireGuardError("");
    } catch (reason) {
      setWireGuardError(reason instanceof Error ? reason.message : t("requestFailed"));
    }
  }, [props.canReadWireGuard, t]);

  useEffect(() => {
    void refreshWireGuard();
  }, [refreshWireGuard]);

  async function runNetworkOperation(
    key: string,
    input: Parameters<typeof api.networkOperation>[0],
  ) {
    setBusy(key);
    setOperationError("");
    setMessage("");
    try {
      const result = await api.networkOperation(input);
      setMessage(result.message);
      props.onChanged();
      await refreshWireGuard();
    } catch (reason) {
      setOperationError(reason instanceof Error ? reason.message : t("requestFailed"));
    } finally {
      setBusy("");
    }
  }

  async function setLink(iface: NetworkInterface, up: boolean) {
    if (!up && !window.confirm(t("networkDisconnectWarning").replace("{interface}", iface.name))) {
      return;
    }
    await runNetworkOperation(`link-${iface.name}`, {
      operation: up ? "link.up" : "link.down",
      interface: iface.name,
    });
  }

  async function changeMTU(iface: NetworkInterface) {
    const raw = window.prompt(t("networkMtuPrompt").replace("{interface}", iface.name), String(iface.mtu));
    if (raw === null) return;
    const mtu = Number(raw);
    if (!Number.isInteger(mtu) || mtu < 576 || mtu > 65535) {
      setOperationError(t("networkInvalidMtu"));
      return;
    }
    await runNetworkOperation(`mtu-${iface.name}`, {
      operation: "mtu",
      interface: iface.name,
      mtu,
    });
  }

  async function addAddress(iface: NetworkInterface) {
    const address = window.prompt(t("networkAddressAddPrompt").replace("{interface}", iface.name), "");
    if (!address) return;
    await runNetworkOperation(`address-add-${iface.name}`, {
      operation: "address.add",
      interface: iface.name,
      address,
    });
  }

  async function deleteAddress(iface: NetworkInterface) {
    const address = window.prompt(
      t("networkAddressDeletePrompt").replace("{interface}", iface.name),
      iface.addresses[0] || "",
    );
    if (!address) return;
    if (!window.confirm(t("networkAddressDeleteConfirm").replace("{address}", address))) return;
    await runNetworkOperation(`address-delete-${iface.name}`, {
      operation: "address.delete",
      interface: iface.name,
      address,
    });
  }

  async function setGateway(iface: NetworkInterface) {
    const gateway = window.prompt(t("networkGatewayPrompt").replace("{interface}", iface.name), "");
    if (!gateway) return;
    await runNetworkOperation(`gateway-${iface.name}`, {
      operation: "gateway.set",
      interface: iface.name,
      gateway,
    });
  }

  async function deleteGateway(iface: NetworkInterface) {
    if (!window.confirm(t("networkGatewayDeleteConfirm").replace("{interface}", iface.name))) return;
    await runNetworkOperation(`gateway-delete-${iface.name}`, {
      operation: "gateway.delete",
      interface: iface.name,
    });
  }

  async function createTunnel(event: FormEvent) {
    event.preventDefault();
    const port = Number(listenPort);
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      setOperationError(t("wireGuardInvalidPort"));
      return;
    }
    await runNetworkOperation("wireguard-create", {
      operation: "wireguard.create",
      tunnel: tunnelName,
      address: tunnelAddress,
      listen_port: port,
    });
  }

  async function setTunnelState(tunnel: WireGuardTunnel, up: boolean) {
    await runNetworkOperation(`wireguard-${tunnel.name}`, {
      operation: up ? "wireguard.up" : "wireguard.down",
      tunnel: tunnel.name,
    });
  }

  async function deleteTunnel(tunnel: WireGuardTunnel) {
    if (!window.confirm(t("wireGuardDeleteConfirm").replace("{tunnel}", tunnel.name))) return;
    await runNetworkOperation(`wireguard-delete-${tunnel.name}`, {
      operation: "wireguard.delete",
      tunnel: tunnel.name,
    });
  }

  async function addPeer(tunnel: WireGuardTunnel) {
    const publicKey = window.prompt(t("wireGuardPeerPublicKey"), "");
    if (!publicKey) return;
    const allowedRaw = window.prompt(t("wireGuardAllowedIpsPrompt"), "");
    if (!allowedRaw) return;
    const endpoint = window.prompt(t("wireGuardEndpointPrompt"), "") || "";
    const keepaliveRaw = window.prompt(t("wireGuardKeepalivePrompt"), "25");
    if (keepaliveRaw === null) return;
    const keepalive = Number(keepaliveRaw || "0");
    if (!Number.isInteger(keepalive) || keepalive < 0 || keepalive > 65535) {
      setOperationError(t("wireGuardInvalidKeepalive"));
      return;
    }
    await runNetworkOperation(`wireguard-peer-${tunnel.name}`, {
      operation: "wireguard.peer.add",
      tunnel: tunnel.name,
      peer_public_key: publicKey,
      allowed_ips: allowedRaw.split(",").map((value) => value.trim()).filter(Boolean),
      endpoint,
      keepalive,
    });
  }

  async function deletePeer(tunnel: WireGuardTunnel, publicKey: string) {
    if (!window.confirm(t("wireGuardPeerDeleteConfirm"))) return;
    await runNetworkOperation(`wireguard-peer-delete-${tunnel.name}`, {
      operation: "wireguard.peer.delete",
      tunnel: tunnel.name,
      peer_public_key: publicKey,
    });
  }

  return (
    <>
      <Panel title={t("networkInterfaces")} className="wide">
        <div className="notice network-runtime-notice">{t("networkRuntimeNotice")}</div>
        {operationError && <div className="form-error">{operationError}</div>}
        {message && <div className="storage-success">{message}</div>}
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("name")}</th>
                <th>{t("state")}</th>
                <th>MAC</th>
                <th>MTU</th>
                <th>{t("addresses")}</th>
                <th>{t("speed")}</th>
                {props.canManage && <th>{t("actions")}</th>}
              </tr>
            </thead>
            <tbody>
              {props.interfaces.map((iface) => (
                <tr key={iface.name}>
                  <td className="mono">{iface.name}</td>
                  <td><Status value={iface.oper_state || (iface.up ? "up" : "down")} /></td>
                  <td className="mono">{iface.mac || "—"}</td>
                  <td>{iface.mtu}</td>
                  <td className="mono">{iface.addresses.join(", ") || "—"}</td>
                  <td>{iface.speed_bps ? `${iface.speed_bps / 1_000_000} Mbps` : "—"}</td>
                  {props.canManage && (
                    <td>
                      <div className="network-actions">
                        {!iface.loopback && (
                          <>
                            <button
                              className="button compact secondary"
                              disabled={busy !== ""}
                              onClick={() => void setLink(iface, !iface.up)}
                            >
                              {iface.up ? t("networkDisable") : t("networkEnable")}
                            </button>
                            <button className="button compact secondary" disabled={busy !== ""} onClick={() => void changeMTU(iface)}>
                              MTU
                            </button>
                            <button className="button compact secondary" disabled={busy !== ""} onClick={() => void addAddress(iface)}>
                              {t("networkAddIp")}
                            </button>
                            <button className="button compact secondary" disabled={busy !== ""} onClick={() => void deleteAddress(iface)}>
                              {t("networkDeleteIp")}
                            </button>
                            <button className="button compact secondary" disabled={busy !== ""} onClick={() => void setGateway(iface)}>
                              {t("networkSetGateway")}
                            </button>
                            <button className="button compact danger" disabled={busy !== ""} onClick={() => void deleteGateway(iface)}>
                              {t("networkDeleteGateway")}
                            </button>
                          </>
                        )}
                      </div>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>

      {props.canReadWireGuard && (
        <Panel title="WireGuard" className="wide">
          {wireGuardError && <div className="form-error">{wireGuardError}</div>}
          {!wireGuard && !wireGuardError && <span className="muted">{t("loading")}</span>}
          {wireGuard && !wireGuard.available && (
            <div className="wireguard-unavailable">
              <div className="notice">{wireGuard.error || t("wireGuardUnavailable")}</div>
              {props.canManage && (
                <button
                  className="button primary"
                  disabled={busy !== ""}
                  onClick={() => void runNetworkOperation("wireguard-install", {operation: "wireguard.install"})}
                >
                  {busy === "wireguard-install" ? t("working") : t("wireGuardInstall")}
                </button>
              )}
            </div>
          )}

          {wireGuard?.available && (
            <div className="wireguard-stack">
              {props.canManage && (
                <form className="wireguard-create" onSubmit={createTunnel}>
                  <label>
                    {t("wireGuardTunnelName")}
                    <input value={tunnelName} onChange={(event) => setTunnelName(event.target.value)} required maxLength={15} />
                  </label>
                  <label>
                    {t("wireGuardAddress")}
                    <input value={tunnelAddress} onChange={(event) => setTunnelAddress(event.target.value)} required />
                  </label>
                  <label>
                    {t("wireGuardListenPort")}
                    <input type="number" min="1" max="65535" value={listenPort} onChange={(event) => setListenPort(event.target.value)} required />
                  </label>
                  <button className="button primary" disabled={busy !== ""} type="submit">
                    {t("wireGuardCreateTunnel")}
                  </button>
                </form>
              )}

              {(wireGuard.tunnels || []).map((tunnel) => (
                <section className="wireguard-tunnel" key={tunnel.name}>
                  <div className="wireguard-tunnel-head">
                    <div>
                      <strong>{tunnel.name}</strong>
                      <span>{tunnel.address || "—"} · UDP {tunnel.listen_port || "—"}</span>
                    </div>
                    <Status value={tunnel.active ? "active" : "disabled"} />
                  </div>

                  <dl className="details wireguard-details">
                    <dt>{t("wireGuardPublicKey")}</dt>
                    <dd className="mono">{tunnel.public_key || "—"}</dd>
                  </dl>

                  {props.canManage && (
                    <div className="network-actions">
                      <button className="button compact secondary" disabled={busy !== ""} onClick={() => void setTunnelState(tunnel, !tunnel.active)}>
                        {tunnel.active ? t("wireGuardDown") : t("wireGuardUp")}
                      </button>
                      <button className="button compact secondary" disabled={busy !== ""} onClick={() => void addPeer(tunnel)}>
                        {t("wireGuardAddPeer")}
                      </button>
                      <button className="button compact danger" disabled={busy !== ""} onClick={() => void deleteTunnel(tunnel)}>
                        {t("wireGuardDeleteTunnel")}
                      </button>
                    </div>
                  )}

                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>{t("wireGuardPeer")}</th>
                          <th>{t("addresses")}</th>
                          <th>{t("wireGuardEndpoint")}</th>
                          <th>{t("wireGuardHandshake")}</th>
                          <th>RX / TX</th>
                          {props.canManage && <th>{t("actions")}</th>}
                        </tr>
                      </thead>
                      <tbody>
                        {(tunnel.peers || []).map((peer) => (
                          <tr key={peer.public_key}>
                            <td className="mono">{peer.public_key}</td>
                            <td className="mono">{(peer.allowed_ips || []).join(", ") || "—"}</td>
                            <td>{peer.endpoint || "—"}</td>
                            <td>{peer.latest_handshake ? date(new Date(peer.latest_handshake * 1000).toISOString()) : "—"}</td>
                            <td>{formatTraffic(peer.transfer_rx)} / {formatTraffic(peer.transfer_tx)}</td>
                            {props.canManage && (
                              <td>
                                <button className="button compact danger" disabled={busy !== ""} onClick={() => void deletePeer(tunnel, peer.public_key)}>
                                  {t("delete")}
                                </button>
                              </td>
                            )}
                          </tr>
                        ))}
                        {(tunnel.peers || []).length === 0 && (
                          <tr><td colSpan={props.canManage ? 6 : 5} className="muted">{t("wireGuardNoPeers")}</td></tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                </section>
              ))}

              {(wireGuard.tunnels || []).length === 0 && <div className="empty-state">{t("wireGuardNoTunnels")}</div>}
            </div>
          )}
        </Panel>
      )}
    </>
  );
}

function formatTraffic(value = 0): string {
  const units = ["B", "KiB", "MiB", "GiB"];
  let current = value;
  let index = 0;
  while (current >= 1024 && index < units.length - 1) {
    current /= 1024;
    index++;
  }
  return `${current.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}
