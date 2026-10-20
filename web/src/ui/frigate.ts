// Never embed an arbitrary URL or run the upstream application on Core's origin.
export function frigateUIURL(coreURL: string, gatewayURL: string): string {
  const core = new URL(coreURL);
  const gateway = new URL(gatewayURL);
  const loopback = core.hostname === "127.0.0.1" || core.hostname === "[::1]";
  if (gateway.protocol !== "https:" && !(loopback && core.protocol === "http:" && gateway.protocol === "http:")) {
    throw new Error("Для интерфейса Frigate требуется HTTPS.");
  }
  if (gateway.protocol !== core.protocol || gateway.hostname !== core.hostname || gateway.origin === core.origin ||
      gateway.username || gateway.password || gateway.pathname !== "/" || gateway.search || gateway.hash) {
    throw new Error("Интерфейс Frigate должен использовать отдельный порт того же сервера, без токенов в URL.");
  }
  return gateway.href;
}
