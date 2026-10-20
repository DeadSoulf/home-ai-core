// Home Assistant has its own origin/authentication, not a token-bearing iframe.
export function homeAssistantURL(coreURL: string): string {
  const url = new URL(coreURL);
  if (url.protocol !== "https:" && url.protocol !== "http:") throw new Error("Unsupported Core URL");
  url.protocol = "http:";
  url.port = "8123";
  url.username = "";
  url.password = "";
  url.pathname = "/";
  url.search = "";
  url.hash = "";
  return url.href;
}
