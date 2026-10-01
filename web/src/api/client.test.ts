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
          reserve_percent: 5,
          warning_percent: 10,
          capacity_known: true,
          size_bytes: 1000,
          free_bytes: 700,
          reserve_bytes: 50,
          warning_bytes: 100,
          capacity_state: "ok",
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

    await api.updateFilePoolCapacityPolicy("nsp-1", {reservePercent: 7, warningPercent: 15});
    const [policyInput, policyInit] = fetchMock.mock.calls[2];
    expect(String(policyInput)).toBe("/api/v1/files/pools/nsp-1/capacity-policy");
    expect(policyInit?.method).toBe("PATCH");
    expect(new Headers(policyInit?.headers).get("X-CSRF-Token")).toBe("csrf-files");
    expect(String(policyInit?.body)).toContain('"reserve_percent":7');
    expect(String(policyInit?.body)).toContain('"warning_percent":15');

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


  it("lists restores and permanently deletes recycle-bin entries", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/trash") && !init?.method) {
        return new Response(JSON.stringify({
          trash: [{
            id: "0123456789abcdef0123456789abcdef",
            original_path: "docs/old.txt",
            name: "old.txt",
            kind: "file",
            size_bytes: 3,
            deleted_at: "2026-09-30T12:00:00Z",
          }],
        }), {
          status: 200,
          headers: {"Content-Type": "application/json"},
        });
      }
      if (init?.method === "DELETE") {
        return new Response(null, {status: 204});
      }
      return new Response(JSON.stringify({path: "docs/old.txt"}), {
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

    const trash = await api.fileTrash("nsf-test");
    expect(trash[0].original_path).toBe("docs/old.txt");

    setCSRFToken("csrf-trash");
    await api.restoreFileTrash("nsf-test", trash[0].id);
    await api.purgeFileTrash("nsf-test", trash[0].id);

    const [, restoreInit] = fetchMock.mock.calls[1];
    expect(restoreInit?.method).toBe("POST");
    expect(new Headers(restoreInit?.headers).get("X-CSRF-Token")).toBe("csrf-trash");

    const [, purgeInit] = fetchMock.mock.calls[2];
    expect(purgeInit?.method).toBe("DELETE");
    expect(new Headers(purgeInit?.headers).get("X-CSRF-Token")).toBe("csrf-trash");

    vi.unstubAllGlobals();
  });


  it("creates, appends and completes a resumable file upload", async () => {
    const upload = {
      id: "0123456789abcdef0123456789abcdef",
      path: "large.bin",
      total_bytes: 5,
      received_bytes: 0,
      client_fingerprint: "large.bin:5:1",
      chunks: [],
      created_at: "2026-09-30T12:00:00Z",
      updated_at: "2026-09-30T12:00:00Z",
    };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/uploads") && init?.method === "POST") {
        return new Response(JSON.stringify({upload}), {
          status: 201,
          headers: {"Content-Type": "application/json"},
        });
      }
      if (url.endsWith("/chunk")) {
        return new Response(JSON.stringify({
          upload: {
            ...upload,
            received_bytes: 5,
            chunks: [{offset: 0, size: 5, sha256: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"}],
          },
        }), {
          status: 200,
          headers: {"Content-Type": "application/json"},
        });
      }
      if (url.endsWith("/complete")) {
        return new Response(JSON.stringify({
          file: {
            path: "large.bin",
            size_bytes: 5,
            sha256: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
          },
        }), {
          status: 200,
          headers: {"Content-Type": "application/json"},
        });
      }
      return new Response(JSON.stringify({uploads: [upload]}), {
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

    setCSRFToken("csrf-resume");
    const listed = await api.fileUploads("nsf-test");
    expect(listed).toHaveLength(1);

    const created = await api.createFileUpload("nsf-test", {
      path: "large.bin",
      totalBytes: 5,
      clientFingerprint: "large.bin:5:1",
    });
    const chunked = await api.uploadFileChunk(
      "nsf-test",
      created.id,
      0,
      new Blob(["hello"]),
      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
    );
    expect(chunked.received_bytes).toBe(5);

    const completed = await api.completeFileUpload("nsf-test", created.id);
    expect(completed.sha256).toBe("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824");

    const [, createInit] = fetchMock.mock.calls[1];
    expect(createInit?.method).toBe("POST");
    expect(new Headers(createInit?.headers).get("X-CSRF-Token")).toBe("csrf-resume");

    const [, chunkInit] = fetchMock.mock.calls[2];
    expect(chunkInit?.method).toBe("PUT");
    expect(new Headers(chunkInit?.headers).get("Upload-Offset")).toBe("0");
    expect(new Headers(chunkInit?.headers).get("X-Chunk-SHA256")).toContain("2cf24dba");
    expect(new Headers(chunkInit?.headers).get("X-CSRF-Token")).toBe("csrf-resume");

    vi.unstubAllGlobals();
  });


  it("reads SMB status and sends protected SMB operations", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v1/files/smb" && !init?.method) {
        return new Response(JSON.stringify({
          smb: {
            available: true,
            active: true,
            hostname: "home-ai",
            workgroup: "WORKGROUP",
            users: [{
              user_id: "usr-test",
              username: "alice",
              display_name: "Alice",
              smb_username: "hai_0123456789abcdef0123",
              configured: false,
            }],
            shares: [{
              folder_id: "nsf-test",
              folder_name: "Family",
              kind: "shared",
              share_name: "HA_Family_12345678",
              unc: "\\\\home-ai\\HA_Family_12345678",
            }],
          },
        }), {
          status: 200,
          headers: {"Content-Type": "application/json"},
        });
      }
      return new Response(JSON.stringify({message: "SMB updated"}), {
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

    const status = await api.smbStatus();
    expect(status.active).toBe(true);
    expect(status.shares[0].unc).toContain("HA_Family");

    setCSRFToken("csrf-smb");
    await api.smbOperation({
      operation: "set_password",
      userId: "usr-test",
      password: "long-smb-password",
    });

    const [input, init] = fetchMock.mock.calls[1];
    expect(String(input)).toBe("/api/v1/files/smb/operation");
    expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("csrf-smb");
    expect(String(init?.body)).toContain('"operation":"set_password"');
    expect(String(init?.body)).toContain('"user_id":"usr-test"');

    vi.unstubAllGlobals();
  });

});
