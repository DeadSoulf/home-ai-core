import type {
  Actor,
  AuditEntry,
  AuthResponse,
  Job,
  RealtimeEvent,
  RegisteredModule, ModuleNavigationState,
  SetupStatus,
  SystemResponse,
  UpdateStatus,
  UpdaterState,
  UserAccount,
  UserProfile,
  PermissionScope,
  AccessCatalog,
  NetworkProfileStatus,
  FileEntry,
  FileFolder,
  FileTrashEntry,
  FileUploadResult,
  FileUploadSession,
  FilePool,
  StoragePurpose,
  StoragePurposeAssignment,
  SMBStatus,
  WireGuardStatus,
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

async function mutateJSON<T>(path: string, method: string, value: unknown): Promise<T> {
  const headers = new Headers({"Content-Type": "application/json"});
  const token = getCSRFToken();
  if (token) headers.set("X-CSRF-Token", token);
  return request<T>(path, {method, headers, body: JSON.stringify(value)});
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

async function requestBlob(path: string): Promise<Blob> {
  const response = await fetch(path, {
    credentials: "same-origin",
    cache: "no-store",
  });
  if (!response.ok) {
    let body: unknown = {};
    const contentType = response.headers.get("Content-Type") || "";
    if (contentType.includes("application/json")) {
      body = await response.json();
    }
    throw new APIError(response.status, body as APIErrorBody);
  }
  return response.blob();
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
    username: string;
    displayName: string;
    password: string;
  }) => {
    const result = await request<AuthResponse>("/api/v1/security/bootstrap", {
      method: "POST",
      headers: new Headers({"Content-Type": "application/json"}),
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

  changePassword: async (currentPassword: string, password: string) => {
    await mutateJSON<void>("/api/v1/auth/password", "PUT", {current_password: currentPassword, password});
    setCSRFToken();
  },
  updateUserIdentity: async (id: string, username: string, displayName: string) => {
    return mutateJSON<{user: UserAccount}>(`/api/v1/security/users/${encodeURIComponent(id)}/identity`, "PUT", {username, display_name: displayName});
  },
  resetUserPassword: async (id: string, password: string) => {
    await mutateJSON<void>(`/api/v1/security/users/${encodeURIComponent(id)}/password`, "PUT", {password});
  },
  fileOwners: async () => (await request<{users: {id: string; username: string; display_name: string}[]}>("/api/v1/files/owners")).users,
  fileFolderSettings: async (id: string) => request<{folder: FileFolder; access: {user_id: string; read: boolean; write: boolean}[]}>(`/api/v1/files/folders/${encodeURIComponent(id)}/settings`),
  updateFileFolderSettings: async (id: string, value: {name: string; quota_bytes: number; enforce_smb: boolean; access: {user_id: string; read: boolean; write: boolean}[]}) => mutateJSON<{warning?: string}>(`/api/v1/files/folders/${encodeURIComponent(id)}/settings`, "PUT", value),
  fileUserQuotas: async () => (await request<{quotas: {user_id: string; username: string; display_name: string; quota_bytes: number; used_bytes: number; reserved_bytes: number; usage_known: boolean}[]}>("/api/v1/files/quotas")).quotas,
  updateUserQuota: async (id: string, quotaBytes: number) => mutateJSON<{warning?: string}>(`/api/v1/files/users/${encodeURIComponent(id)}/quota`, "PUT", {quota_bytes: quotaBytes}),

  users: async () => {
    const result = await request<{users: UserAccount[]}>("/api/v1/security/users");
    return result.users;
  },

  userAccessCatalog: async () => {
    const result = await request<{access: AccessCatalog}>("/api/v1/security/access-catalog");
    return result.access;
  },

  createUser: async (input: {
    username: string;
    displayName: string;
    password: string;
    profile?: UserProfile;
    permissions?: string[];
    resourcePermissions?: PermissionScope[];
  }) => {
    const result = await postJSON<{user: UserAccount}>("/api/v1/security/users", {
      username: input.username,
      display_name: input.displayName,
      password: input.password,
      profile: input.profile,
      permissions: input.permissions || [],
      resource_permissions: input.resourcePermissions || [],
    }, true);
    return result.user;
  },

  updateUserAccess: async (userId: string, input: {
    profile: UserProfile;
    permissions: string[];
    resourcePermissions?: PermissionScope[];
    disabled: boolean;
  }) => {
    const headers = new Headers({"Content-Type": "application/json"});
    const token = getCSRFToken();
    if (token) headers.set("X-CSRF-Token", token);
    const result = await request<{user: UserAccount; warning?: string}>(
      `/api/v1/security/users/${encodeURIComponent(userId)}/access`,
      {
        method: "PUT",
        headers,
        body: JSON.stringify({
          profile: input.profile,
          permissions: input.permissions,
          resource_permissions: input.resourcePermissions || [],
          disabled: input.disabled,
        }),
      },
    );
    return {...result.user, warning: result.warning};
  },

  storagePurposes: async () => {
    const result = await request<{assignments: StoragePurposeAssignment[]}>("/api/v1/storage/purposes");
    return result.assignments;
  },

  setStoragePurpose: async (device: string, purpose?: StoragePurpose) => {
    const result = await postJSON<{assignment?: StoragePurposeAssignment}>(
      "/api/v1/storage/purposes",
      {device, purpose: purpose || "none"},
      true,
    );
    return result.assignment;
  },

  filePools: async () => {
    const result = await request<{pools: FilePool[]}>("/api/v1/files/pools");
    return result.pools;
  },

  createFilePool: async (input: {name: string; rootPath: string}) => {
    const result = await postJSON<{pool: FilePool}>("/api/v1/files/pools", {
      name: input.name,
      root_path: input.rootPath,
    }, true);
    return result.pool;
  },

  updateFilePoolCapacityPolicy: async (
    poolId: string,
    input: {reservePercent: number; warningPercent: number},
  ) => {
    const headers = new Headers({"Content-Type": "application/json"});
    const token = getCSRFToken();
    if (token) headers.set("X-CSRF-Token", token);
    const result = await request<{pool: FilePool}>(
      `/api/v1/files/pools/${encodeURIComponent(poolId)}/capacity-policy`,
      {
        method: "PATCH",
        headers,
        body: JSON.stringify({
          reserve_percent: input.reservePercent,
          warning_percent: input.warningPercent,
        }),
      },
    );
    return result.pool;
  },

  smbStatus: async () => {
    const result = await request<{smb: SMBStatus}>("/api/v1/files/smb");
    return result.smb;
  },

  smbOperation: async (input: {
    operation: "install" | "set_password" | "apply";
    userId?: string;
    password?: string;
    workgroup?: string;
  }) => postJSON<{message: string}>("/api/v1/files/smb/operation", {
    operation: input.operation,
    user_id: input.userId,
    password: input.password,
    workgroup: input.workgroup,
  }, true),

  fileFolders: async () => {
    const result = await request<{folders: FileFolder[]}>("/api/v1/files/folders");
    return result.folders;
  },

  createFileFolder: async (input: {
    poolId: string;
    name: string;
    kind: "private" | "shared";
    ownerUserId?: string;
  }) => {
    const result = await postJSON<{folder: FileFolder; warning?: string}>("/api/v1/files/folders", {
      pool_id: input.poolId,
      name: input.name,
      kind: input.kind,
      owner_user_id: input.ownerUserId,
    }, true);
    return {...result.folder, warning: result.warning};
  },

  fileEntries: async (folderId: string, path = "") => {
    const query = new URLSearchParams();
    if (path) query.set("path", path);
    const suffix = query.size ? `?${query.toString()}` : "";
    const result = await request<{entries: FileEntry[]}>(`/api/v1/files/folders/${encodeURIComponent(folderId)}/entries${suffix}`);
    return result.entries;
  },

  createFileDirectory: async (folderId: string, path: string) => {
    return postJSON<{path: string}>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/directories`,
      {path},
      true,
    );
  },

  fileUploads: async (folderId: string) => {
    const result = await request<{uploads: FileUploadSession[]}>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/uploads`,
    );
    return result.uploads;
  },

  createFileUpload: async (folderId: string, input: {
    path: string;
    totalBytes: number;
    sha256?: string;
    clientFingerprint?: string;
  }) => {
    const result = await postJSON<{upload: FileUploadSession}>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/uploads`,
      {
        path: input.path,
        total_bytes: input.totalBytes,
        sha256: input.sha256,
        client_fingerprint: input.clientFingerprint,
      },
      true,
    );
    return result.upload;
  },

  fileUpload: async (folderId: string, uploadId: string) => {
    const result = await request<{upload: FileUploadSession}>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/uploads/${encodeURIComponent(uploadId)}`,
    );
    return result.upload;
  },

  uploadFileChunk: async (
    folderId: string,
    uploadId: string,
    offset: number,
    chunk: Blob,
    sha256?: string,
  ) => {
    const headers = new Headers({
      "Content-Type": "application/octet-stream",
      "Upload-Offset": String(offset),
    });
    const token = getCSRFToken();
    if (token) headers.set("X-CSRF-Token", token);
    if (sha256) headers.set("X-Chunk-SHA256", sha256);
    const result = await request<{upload: FileUploadSession}>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/uploads/${encodeURIComponent(uploadId)}/chunk`,
      {method: "PUT", headers, body: chunk},
    );
    return result.upload;
  },

  completeFileUpload: async (folderId: string, uploadId: string) => {
    const result = await postJSON<{file: FileUploadResult}>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/uploads/${encodeURIComponent(uploadId)}/complete`,
      undefined,
      true,
    );
    return result.file;
  },

  cancelFileUpload: async (folderId: string, uploadId: string) => {
    const token = getCSRFToken();
    const headers = new Headers();
    if (token) headers.set("X-CSRF-Token", token);
    return request<void>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/uploads/${encodeURIComponent(uploadId)}`,
      {method: "DELETE", headers},
    );
  },

  uploadFile: async (folderId: string, path: string, file: File) => {
    const token = getCSRFToken();
    const headers = new Headers({"Content-Type": file.type || "application/octet-stream"});
    if (token) headers.set("X-CSRF-Token", token);
    const query = new URLSearchParams({path});
    return request<{path: string; size_bytes: number}>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/content?${query.toString()}`,
      {method: "PUT", headers, body: file},
    );
  },

  downloadFile: async (folderId: string, path: string) => {
    const query = new URLSearchParams({path});
    return requestBlob(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/content?${query.toString()}`,
    );
  },

  deleteFileEntry: async (folderId: string, path: string) => {
    const token = getCSRFToken();
    const headers = new Headers();
    if (token) headers.set("X-CSRF-Token", token);
    const query = new URLSearchParams({path});
    return request<void>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/entry?${query.toString()}`,
      {method: "DELETE", headers},
    );
  },

  moveFileEntry: async (folderId: string, fromPath: string, toPath: string) => {
    return postJSON<{path: string}>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/move`,
      {from_path: fromPath, to_path: toPath},
      true,
    );
  },

  fileTrash: async (folderId: string) => {
    const result = await request<{trash: FileTrashEntry[]}>(`/api/v1/files/folders/${encodeURIComponent(folderId)}/trash`);
    return result.trash;
  },

  restoreFileTrash: async (folderId: string, trashId: string) => {
    return postJSON<{path: string}>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/trash/${encodeURIComponent(trashId)}/restore`,
      undefined,
      true,
    );
  },

  purgeFileTrash: async (folderId: string, trashId: string) => {
    const token = getCSRFToken();
    const headers = new Headers();
    if (token) headers.set("X-CSRF-Token", token);
    return request<void>(
      `/api/v1/files/folders/${encodeURIComponent(folderId)}/trash/${encodeURIComponent(trashId)}`,
      {method: "DELETE", headers},
    );
  },

  system: () => request<SystemResponse>("/api/v1/system"),

  setDiskName: async (device: string, name: string) =>
    postJSON<{name: string}>("/api/v1/storage/name", {device, name}, true),

  storageOperation: async (input: {
    operation: "mount" | "unmount" | "format" | "partition.create" | "partition.delete" | "partition.delete_all" | "label.rename" | "preflight";
    device: string;
    mountpoint?: string;
    filesystem?: "ext4" | "xfs" | "vfat";
    label?: string;
    confirm?: string;
    dry_run?: boolean;
    size_mib?: number;
    purpose?: StoragePurpose;
  }) => postJSON<{
    message: string;
    mountpoint?: string;
    device?: string;
    purpose?: StoragePurpose;
    purpose_assigned?: boolean;
    warning?: string;
    dry_run?: boolean;
  }>("/api/v1/storage/operation", input, true),

  networkProfiles: async () => {
    const result = await request<{network_profiles: NetworkProfileStatus}>("/api/v1/network/profiles");
    return result.network_profiles;
  },

  wireGuardStatus: async () => {
    const result = await request<{wireguard: WireGuardStatus}>("/api/v1/network/wireguard");
    return result.wireguard;
  },

  networkOperation: async (input: {
    operation:
      | "link.up"
      | "link.down"
      | "mtu"
      | "address.add"
      | "address.delete"
      | "gateway.set"
      | "gateway.delete"
      | "profile.save"
      | "wireguard.install"
      | "wireguard.create"
      | "wireguard.up"
      | "wireguard.down"
      | "wireguard.delete"
      | "wireguard.peer.add"
      | "wireguard.peer.delete";
    interface?: string;
    address?: string;
    gateway?: string;
    mtu?: number;
    tunnel?: string;
    listen_port?: number;
    private_key?: string;
    peer_public_key?: string;
    preshared_key?: string;
    allowed_ips?: string[];
    endpoint?: string;
    keepalive?: number;
    network_method?: "dhcp" | "static";
    dns?: string[];
  }) => postJSON<{message: string}>("/api/v1/network/operation", input, true),

  updateStatus: async (fresh = false) => {
    const result = await request<{update: UpdateStatus}>(fresh ? "/api/v1/update?fresh=1" : "/api/v1/update");
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

  moduleNavigation: async () => {
    const result = await request<{modules: ModuleNavigationState[]}>("/api/v1/modules/navigation");
    return result.modules;
  },

  capabilities: async () => {
    const result = await request<{capabilities: string[]}>("/api/v1/modules/capabilities");
    return result.capabilities;
  },

  controlModule: async (id: string, operation: "enable" | "disable" | "restart") => {
    const result = await postJSON<{module: RegisteredModule}>(
      `/api/v1/modules/${encodeURIComponent(id)}/control`,
      {operation},
      true,
    );
    return result.module;
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
