import {afterEach, describe, expect, it, vi} from "vitest";
import {api, setCSRFToken} from "./client";
import type {RegisteredModule} from "./types";

const approved: RegisteredModule["manifest"] = {
  schema_version: 2, id: "frigate.module", name: "Frigate NVR", version: "0.18.2", core: ">=0.1.211 <1.0.0",
  runtime: {type: "docker", docker: {image: `ghcr.io/blakeblackshear/frigate@sha256:${"a".repeat(64)}`}},
  lifecycle: ["install", "upgrade", "remove", "start", "stop", "restart"],
};
function setup(response: unknown = {job: {id: "job-frigate", status: "queued"}}) {
  const storage = new Map<string,string>();
  vi.stubGlobal("localStorage", {getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value), removeItem: (key: string) => storage.delete(key)});
  setCSRFToken("csrf-frigate");
  const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify(response), {status: 200, headers: {"Content-Type": "application/json"}}));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
afterEach(() => vi.unstubAllGlobals());
describe("Frigate manual approval and native gateway", () => {
  it("sends the exact approved immutable image and version for installation", async () => {
    const fetchMock = setup();
    await api.installCatalogModule("frigate.module", approved);
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/modules/catalog/frigate.module/install");
    expect(JSON.parse(String(init?.body))).toEqual({expected_version: approved.version, expected_image: approved.runtime.docker.image});
    expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf-frigate");
    expect(init?.credentials).toBe("same-origin");
  });
  it("does not silently replace the approved digest at upgrade submission", async () => {
    const fetchMock = setup();
    await api.upgradeCatalogModule("frigate.module", approved);
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({expected_version: "0.18.2", expected_image: approved.runtime.docker.image});
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
  it("keeps existing NVR catalog actions independent", async () => {
    const fetchMock = setup();
    await api.installCatalogModule("nvr.module");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/modules/catalog/nvr.module/install");
    expect(fetchMock.mock.calls[0][1]?.body).toBeUndefined();
  });
  it("checks releases without submitting any installation", async () => {
    const fetchMock = setup({catalog: {modules: [], frigate_upstream: {stale: true}}});
    const catalog = await api.moduleCatalog(true);
    expect(catalog.frigate_upstream?.stale).toBe(true);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/modules/catalog?refresh=1");
    expect(fetchMock.mock.calls[0][1]?.method).toBeUndefined();
  });
  it("saves raw YAML through the protected fixed gateway", async () => {
    const fetchMock = setup({success: true});
    await api.frigateSaveConfig("mqtt:\n  enabled: false\n");
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/frigate/api/config/save");
    expect(init?.body).toBe("mqtt:\n  enabled: false\n");
    expect(new Headers(init?.headers).get("Content-Type")).toBe("text/plain; charset=utf-8");
    expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf-frigate");
  });
});
