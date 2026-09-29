import type {
  Actor,
  AuditEntry,
  AuthResponse,
  Job,
  RealtimeEvent,
  RegisteredModule,
  SetupStatus,
  SystemResponse,
  UpdateStatus,
  UpdaterState,
} from "./types";

type APIErrorBody = {
  error?: {
    code?: string;
    message?: string;
    request_id?: string;
    correlation_id?: string;
  };
};

export class APIError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;
  readonly correlationId?: string;

  constructor(status: number, body: APIErrorBody) {
    const error = body.error;
    super(error?.message || `Request failed with status ${status}`);
    this.name = "APIError";
    this.status = status;
    this.code = error?.code || "request_failed";
    this.requestId = error?.request_id;
    this.correlationId = error?.correlation_id;
  }
}

const csrfStorageKey = "home-ai-core.csrf";

export function getCSRFToken(): string {
  return localStorage.getItem(csrfStorageKey) || "";
}

export function setCSRFToken(value?: string): void {
  if (value) {
    localStorage.setItem(csrfStorageKey, value);
  } else {
    localStorage.removeItem(csrfStorageKey);
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");

  const response = await fetch(path, {
    ...init,
    headers,
    credentials: "same-origin",
    cache: "no-store",
  });

  if (response.status === 204) {
    return undefined as T;
  }

  let body: unknown = {};
  const contentType = response.headers.get("Content-Type") || "";
  if (contentType.includes("application/json")) {
    body = await response.json();
  }

  if (!response.ok) {
    throw new APIError(response.status, body as APIErrorBody);
  }
  return body as T;
}

async function postJSON<T>(path: string, body?: unknown, csrf = false): Promise<T> {
  const headers = new Headers({"Content-Type": "application/json"});
  if (csrf) {
    const token = getCSRFToken();
    if (token) {
      headers.set("X-CSRF-Token", token);
    }
  }
  return request<T>(path, {
    method: "POST",
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export const api = {
  setupStatus: () => request<SetupStatus>("/api/v1/security/setup-status"),

  bootstrap: async (input: {
    bootstrapToken: string;
    username: string;
    displayName: string;
    password: string;
  }) => {
    const headers = new Headers({
      "Content-Type": "application/json",
      "X-Home-AI-Bootstrap-Token": input.bootstrapToken,
    });
    const result = await request<AuthResponse>("/api/v1/security/bootstrap", {
      method: "POST",
      headers,
      body: JSON.stringify({
        username: input.username,
        display_name: input.displayName,
        password: input.password,
        session_mode: "cookie",
      }),
    });
    setCSRFToken(result.csrf_token);
    return result;
  },

  login: async (username: string, password: string) => {
    const result = await postJSON<AuthResponse>("/api/v1/auth/login", {
      username,
      password,
      session_mode: "cookie",
    });
    setCSRFToken(result.csrf_token);
    return result;
  },

  me: async () => {
    const result = await request<{actor: Actor}>("/api/v1/auth/me");
    return result.actor;
  },

  logout: async () => {
    await postJSON<void>("/api/v1/auth/logout", undefined, true);
    setCSRFToken();
  },

  system: () => request<SystemResponse>("/api/v1/system"),

  setDiskName: async (device: string, name: string) =>
    postJSON<{name: string}>("/api/v1/storage/name", {device, name}, true),

  storageOperation: async (input: {
    operation: "mount" | "unmount" | "format" | "partition.create" | "partition.delete" | "partition.delete_all" | "label.rename";
    device: string;
    mountpoint?: string;
    filesystem?: "ext4" | "xfs" | "vfat";
    label?: string;
    confirm?: string;
    size_mib?: number;
  }) => postJSON<{message: string}>("/api/v1/storage/operation", input, true),

  updateStatus: async () => {
    const result = await request<{update: UpdateStatus}>("/api/v1/update");
    return result.update;
  },

  updaterState: async () => {
    const result = await request<{state: UpdaterState}>("/api/v1/update/state");
    return result.state;
  },

  downloadUpdate: async (version: string) => {
    const result = await postJSON<{state: UpdaterState}>("/api/v1/update/download", {version}, true);
    return result.state;
  },

  installUpdate: async (version: string) => {
    const result = await postJSON<{state: UpdaterState}>("/api/v1/update/install", {version}, true);
    return result.state;
  },

  rollbackUpdate: async () => {
    const result = await postJSON<{state: UpdaterState}>("/api/v1/update/rollback", undefined, true);
    return result.state;
  },

  modules: async () => {
    const result = await request<{modules: RegisteredModule[]}>("/api/v1/modules");
    return result.modules;
  },

  capabilities: async () => {
    const result = await request<{capabilities: string[]}>("/api/v1/modules/capabilities");
    return result.capabilities;
  },

  jobs: async () => {
    const result = await request<{jobs: Job[]}>("/api/v1/jobs?limit=100");
    return result.jobs;
  },

  clearJobs: async () => {
    const headers = new Headers();
    const token = getCSRFToken();
    if (token) headers.set("X-CSRF-Token", token);
    return request<{deleted: number}>("/api/v1/jobs", {
      method: "DELETE",
      headers,
    });
  },

  audit: async () => {
    const result = await request<{events: AuditEntry[]}>("/api/v1/audit?limit=100");
    return result.events;
  },
};

export type RealtimeStatus = "connecting" | "connected" | "disconnected";

export function connectRealtime(
  onEvent: (event: RealtimeEvent) => void,
  onStatus: (status: RealtimeStatus) => void,
): () => void {
  let stopped = false;
  let socket: WebSocket | undefined;
  let retry = 1000;
  let timer: number | undefined;

  const connect = () => {
    if (stopped) return;
    onStatus("connecting");

    const url = new URL("/api/v1/events", window.location.href);
    url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    socket = new WebSocket(url);

    socket.onopen = () => {
      retry = 1000;
      onStatus("connected");
      socket?.send(JSON.stringify({
        op: "subscribe",
        topics: ["system.*", "job.*", "module.*", "security.*"],
      }));
    };

    socket.onmessage = (message) => {
      try {
        onEvent(JSON.parse(String(message.data)) as RealtimeEvent);
      } catch {
        // Ignore malformed transport frames. Core protocol errors arrive as events.
      }
    };

    socket.onclose = () => {
      onStatus("disconnected");
      if (!stopped) {
        timer = window.setTimeout(connect, retry);
        retry = Math.min(retry * 2, 10000);
      }
    };

    socket.onerror = () => socket?.close();
  };

  connect();

  return () => {
    stopped = true;
    if (timer !== undefined) window.clearTimeout(timer);
    socket?.close(1000, "ui stopped");
  };
}
