import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, CSRF_FAILURE_EVENT, getCSRFToken, setCSRFToken } from "./client";

const actor = {id: "owner-1", type: "user", username: "owner", permissions: [], roles: []};
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
  status, headers: {"Content-Type": "application/json"},
});

describe("browser session and console CSRF readiness", () => {
  beforeEach(() => {
    const values = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => { values.set(key, value); },
      removeItem: (key: string) => { values.delete(key); },
    });
    setCSRFToken();
  });
  afterEach(() => { setCSRFToken(); vi.unstubAllGlobals(); });

  it("sends the synchronizer token on the read-only session check", async () => {
    setCSRFToken("token-test-only");
    const fetchMock = vi.fn(async (_path: string, _init: RequestInit) => json({actor, csrf_valid: true}));
    vi.stubGlobal("fetch", fetchMock);
    expect(await api.me()).toEqual(actor);
    expect(new Headers(fetchMock.mock.calls[0][1].headers).get("X-CSRF-Token")).toBe("token-test-only");
  });

  it.each(["", "old-origin-token"])("rejects cookie-only restore with stale/missing token %s", async (token) => {
    setCSRFToken(token);
    vi.stubGlobal("fetch", vi.fn(async () => json({actor, csrf_valid: false})));
    await expect(api.me()).rejects.toMatchObject({status: 403, code: "csrf_required"});
  });

  it("does not silently accept an old backend without a readiness response", async () => {
    setCSRFToken("token-test-only");
    vi.stubGlobal("fetch", vi.fn(async () => json({actor})));
    await expect(api.me()).rejects.toMatchObject({code: "csrf_required"});
  });

  it("keeps a fresh token in tab memory when browser storage is blocked", async () => {
    const denied = () => { throw new DOMException("denied", "SecurityError"); };
    vi.stubGlobal("localStorage", {getItem: denied, setItem: denied, removeItem: denied});
    setCSRFToken("memory-token");
    expect(getCSRFToken()).toBe("memory-token");
    const fetchMock = vi.fn(async (_path: string, _init: RequestInit) => json({actor, csrf_valid: true}));
    vi.stubGlobal("fetch", fetchMock);
    await api.me();
    expect(new Headers(fetchMock.mock.calls[0][1].headers).get("X-CSRF-Token")).toBe("memory-token");
    setCSRFToken();
    expect(getCSRFToken()).toBe("");
  });

  it("does not reuse old persisted data after a quota/write failure", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => "obsolete-token",
      setItem: () => { throw new DOMException("full", "QuotaExceededError"); },
      removeItem: () => { throw new DOMException("full", "QuotaExceededError"); },
    });
    setCSRFToken("fresh-memory-token");
    expect(getCSRFToken()).toBe("fresh-memory-token");
  });

  it("confirms the login cookie and CSRF pair before returning authenticated actor", async () => {
    const fetchMock = vi.fn(async (path: string, init: RequestInit) => {
      if (path.endsWith("/login")) return json({actor, csrf_token: "fresh-login-token"});
      expect(new Headers(init.headers).get("X-CSRF-Token")).toBe("fresh-login-token");
      return json({actor, csrf_valid: true});
    });
    vi.stubGlobal("fetch", fetchMock);
    expect((await api.login("owner", "test-password")).actor.id).toBe(actor.id);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("rejects a login when the returned token does not match the browser cookie", async () => {
    vi.stubGlobal("fetch", vi.fn(async (path: string) => path.endsWith("/login")
      ? json({actor, csrf_token: "fresh-login-token"}) : json({actor, csrf_valid: false})));
    await expect(api.login("owner", "test-password")).rejects.toMatchObject({code: "csrf_required"});
  });

  it("does not silently switch to a different account during login confirmation", async () => {
    vi.stubGlobal("fetch", vi.fn(async (path: string) => path.endsWith("/login")
      ? json({actor, csrf_token: "fresh-login-token"}) : json({actor: {...actor, id: "other-user"}, csrf_valid: true})));
    await expect(api.login("owner", "test-password")).rejects.toMatchObject({code: "session_changed"});
  });

  it("signals a rejected console connection without repeating the POST", async () => {
    const events = new EventTarget(); const expired = vi.fn();
    events.addEventListener(CSRF_FAILURE_EVENT, expired); vi.stubGlobal("window", events);
    setCSRFToken("bad-token");
    const fetchMock = vi.fn(async () => json({error: {code: "csrf_required"}}, 403));
    vi.stubGlobal("fetch", fetchMock);
    await expect(api.consoleTicket()).rejects.toMatchObject({code: "csrf_required"});
    expect(expired).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(getCSRFToken()).toBe("bad-token"); // no false logout/revocation
  });

  it("ignores an obsolete request's auth event after another login changed the token", async () => {
    const events = new EventTarget(); const expired = vi.fn();
    events.addEventListener(CSRF_FAILURE_EVENT, expired); vi.stubGlobal("window", events);
    setCSRFToken("old-token");
    vi.stubGlobal("fetch", vi.fn(async () => {
      setCSRFToken("new-token");
      return json({error: {code: "csrf_required"}}, 403);
    }));
    await expect(api.consoleTicket()).rejects.toMatchObject({code: "csrf_required"});
    expect(expired).not.toHaveBeenCalled();
    expect(getCSRFToken()).toBe("new-token");
  });
});
