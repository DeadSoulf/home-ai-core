import { describe, expect, it, vi } from "vitest";
import { APIError, api } from "./client";

describe("API client", () => {
  it("parses the Core error envelope", async () => {
    vi.stubGlobal("fetch", vi.fn(async () =>
      new Response(JSON.stringify({
        error: {
          code: "permission_denied",
          message: "permission denied",
          request_id: "req-1",
        },
      }), {
        status: 403,
        headers: {"Content-Type": "application/json"},
      }),
    ));

    await expect(api.system()).rejects.toMatchObject({
      name: "APIError",
      status: 403,
      code: "permission_denied",
      requestId: "req-1",
    });

    vi.unstubAllGlobals();
  });

  it("uses cookie-mode login and stores only the CSRF token", async () => {
    const fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({
        actor: {
          type: "user",
          id: "usr_1",
          username: "owner",
          roles: ["owner"],
          permissions: ["system.read"],
        },
        expires_at: "2026-09-29T00:00:00Z",
        csrf_token: "csrf-test",
      }), {
        status: 200,
        headers: {"Content-Type": "application/json"},
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });

    const result = await api.login("owner", "secret");
    expect(result.actor.username).toBe("owner");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0] as [unknown, RequestInit];
    expect(init.credentials).toBe("same-origin");
    expect(String(init.body)).toContain('"session_mode":"cookie"');
    expect(storage.get("home-ai-core.csrf")).toBe("csrf-test");

    vi.unstubAllGlobals();
  });
});
