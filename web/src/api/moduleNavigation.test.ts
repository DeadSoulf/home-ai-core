import {afterEach, describe, expect, it, vi} from "vitest";
import {api} from "./client";

afterEach(() => vi.unstubAllGlobals());

describe("module navigation request lifecycle", () => {
  it("loads old installed manifests without navigation and never caches the registry", async () => {
    const fetchMock = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit) =>
      new Response(JSON.stringify({modules: [{module_id: "homeassistant.module", name: "Home Assistant", status: "disabled", items: []}]}),
        {headers: {"Content-Type": "application/json"}}));
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    const modules = await api.moduleNavigation(controller.signal);
    expect(modules[0]).toMatchObject({name: "Home Assistant", status: "disabled", items: []});
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/modules/navigation");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({cache: "no-store", credentials: "same-origin"});
    const signal = fetchMock.mock.calls[0][1]?.signal;
    expect(signal?.aborted).toBe(false);
    controller.abort();
    expect(signal?.aborted).toBe(true);
  });

  it("cancels the previous session's in-flight registry request", async () => {
    vi.stubGlobal("fetch", vi.fn((_url: RequestInfo | URL, init?: RequestInit) => new Promise<Response>((_resolve, reject) => {
      const abort = () => reject(new DOMException("Session ended", "AbortError"));
      if (init?.signal?.aborted) abort();
      else init?.signal?.addEventListener("abort", abort, {once: true});
    })));
    const controller = new AbortController();
    const request = api.moduleNavigation(controller.signal);
    const assertion = expect(request).rejects.toMatchObject({name: "AbortError"});
    controller.abort();
    await assertion;
  });
});
