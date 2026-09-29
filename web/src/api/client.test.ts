import { describe, expect, it, vi } from "vitest";
import { APIError, api, setCSRFToken } from "./client";

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
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
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
    const [, init] = fetchMock.mock.calls[0];
    expect(init?.credentials).toBe("same-origin");
    expect(String(init?.body)).toContain('"session_mode":"cookie"');
    expect(storage.get("home-ai-core.csrf")).toBe("csrf-test");

    vi.unstubAllGlobals();
  });

  it("creates a household user with CSRF protection", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(JSON.stringify({
        user: {
          id: "usr_member",
          username: "alice",
          display_name: "Alice",
          disabled: false,
          created_at: "2026-09-29T00:00:00Z",
          roles: ["member"],
        },
      }), {
        status: 201,
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

    setCSRFToken("csrf-create-user");
    const user = await api.createUser({
      username: "alice",
      displayName: "Alice",
      password: "correct horse battery staple",
    });

    expect(user.roles).toEqual(["member"]);
    const [input, init] = fetchMock.mock.calls[0];
    expect(String(input)).toBe("/api/v1/security/users");
    expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf-create-user");
    expect(String(init?.body)).toContain('"display_name":"Alice"');

    vi.unstubAllGlobals();
  });

  it("sends network changes with CSRF protection", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(JSON.stringify({message: "updated"}), {
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

    setCSRFToken("csrf-network");
    const result = await api.networkOperation({
      operation: "mtu",
      interface: "eth0",
      mtu: 1400,
    });
    expect(result.message).toBe("updated");
    const [input, init] = fetchMock.mock.calls[0];
    expect(String(input)).toBe("/api/v1/network/operation");
    expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf-network");
    expect(String(init?.body)).toContain('"interface":"eth0"');

    vi.unstubAllGlobals();
  });


  it("loads and saves persistent network profiles", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === "/api/v1/network/profiles") {
        return new Response(JSON.stringify({
          network_profiles: {
            backend: "systemd-networkd",
            profiles: [{
              interface: "eth0",
              backend: "systemd-networkd",
              supported: true,
              managed: true,
              method: "dhcp",
              dns: ["1.1.1.1"],
            }],
          },
        }), {
          status: 200,
          headers: {"Content-Type": "application/json"},
        });
      }
      expect(init?.method).toBe("POST");
      return new Response(JSON.stringify({message: "persistent profile applied"}), {
        status: 200,
        headers: {"Content-Type": "application/json"},
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });

    const profiles = await api.networkProfiles();
    expect(profiles.backend).toBe("systemd-networkd");
    expect(profiles.profiles[0].interface).toBe("eth0");

    setCSRFToken("csrf-profile");
    await api.networkOperation({
      operation: "profile.save",
      interface: "eth0",
      network_method: "static",
      address: "192.168.50.10/24",
      gateway: "192.168.50.1",
      dns: ["1.1.1.1"],
    });

    const [input, init] = fetchMock.mock.calls[1];
    expect(String(input)).toBe("/api/v1/network/operation");
    expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf-profile");
    expect(String(init?.body)).toContain('"network_method":"static"');

    vi.unstubAllGlobals();
  });


  it("loads folders and creates a NAS pool with CSRF protection", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === "/api/v1/files/folders") {
        return new Response(JSON.stringify({
          folders: [{
            id: "nsf-1",
            pool_id: "nsp-1",
            pool_name: "Main",
            name: "Family",
            kind: "shared",
            relative_path: "shared/nsf-1",
            can_read: true,
            can_write: true,
          }],
        }), {
          status: 200,
          headers: {"Content-Type": "application/json"},
        });
      }
      return new Response(JSON.stringify({
        pool: {
          id: "nsp-1",
          name: "Main",
          root_path: "/srv/home-ai/main",
        },
      }), {
        status: 201,
        headers: {"Content-Type": "application/json"},
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });

    const folders = await api.fileFolders();
    expect(folders).toHaveLength(1);
    expect(folders[0].name).toBe("Family");

    setCSRFToken("csrf-files");
    const pool = await api.createFilePool({name: "Main", rootPath: "/srv/home-ai/main"});
    expect(pool.id).toBe("nsp-1");

    const [input, init] = fetchMock.mock.calls[1];
    expect(String(input)).toBe("/api/v1/files/pools");
    expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf-files");
    expect(String(init?.body)).toContain('"root_path":"/srv/home-ai/main"');

    vi.unstubAllGlobals();
  });


  it("browses and uploads files in a logical NAS folder", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/entries")) {
        return new Response(JSON.stringify({
          entries: [{
            name: "docs",
            path: "docs",
            kind: "directory",
            modified_at: "2026-09-30T10:00:00Z",
          }],
        }), {
          status: 200,
          headers: {"Content-Type": "application/json"},
        });
      }
      return new Response(JSON.stringify({
        path: "hello.txt",
        size_bytes: 5,
      }), {
        status: 201,
        headers: {"Content-Type": "application/json"},
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });

    const entries = await api.fileEntries("nsf-test");
    expect(entries[0].kind).toBe("directory");

    setCSRFToken("csrf-files");
    const file = new File(["hello"], "hello.txt", {type: "text/plain"});
    await api.uploadFile("nsf-test", "hello.txt", file);

    const [input, init] = fetchMock.mock.calls[1];
    expect(String(input)).toContain("/api/v1/files/folders/nsf-test/content?");
    expect(init?.method).toBe("PUT");
    expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf-files");

    vi.unstubAllGlobals();
  });


  it("moves and deletes NAS entries with CSRF protection", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "DELETE") {
        return new Response(null, {status: 204});
      }
      return new Response(JSON.stringify({path: "docs/new.txt"}), {
        status: 200,
        headers: {"Content-Type": "application/json"},
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });

    setCSRFToken("csrf-file-mutate");
    await api.moveFileEntry("nsf-test", "docs/old.txt", "docs/new.txt");
    await api.deleteFileEntry("nsf-test", "docs/new.txt");

    const [, moveInit] = fetchMock.mock.calls[0];
    expect(moveInit?.method).toBe("POST");
    expect(new Headers(moveInit?.headers).get("X-CSRF-Token")).toBe("csrf-file-mutate");

    const [, deleteInit] = fetchMock.mock.calls[1];
    expect(deleteInit?.method).toBe("DELETE");
    expect(new Headers(deleteInit?.headers).get("X-CSRF-Token")).toBe("csrf-file-mutate");

    vi.unstubAllGlobals();
  });

});
