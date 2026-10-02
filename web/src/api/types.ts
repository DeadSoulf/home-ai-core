export type PermissionScope = {
  permission: string;
  resource_type: string;
  resource_id: string;
};

export type Actor = {
  type: string;
  id: string;
  username?: string;
  display_name?: string;
  roles: string[];
  permissions: string[];
  resource_permissions?: PermissionScope[];
};

export type UserProfile = "administrator" | "parent" | "child" | "guest" | "friend";

export type UserAccount = {
  id: string;
  username: string;
  display_name: string;
  disabled: boolean;
  created_at: string;
  last_login_at?: string;
  roles: string[];
  profile: UserProfile;
  permissions: string[];
  resource_permissions?: PermissionScope[];
};

export type AccessProfile = {
  id: UserProfile;
  description: string;
  full_access: boolean;
  default_permissions: string[];
};

export type PermissionDefinition = {
  name: string;
  description: string;
  category: string;
};

export type AccessResource = {
  type: string;
  id: string;
  name: string;
  description?: string;
  permissions: string[];
  owner_user_id?: string;
};

export type AccessCatalog = {
  profiles: AccessProfile[];
  permissions: PermissionDefinition[];
  resources: AccessResource[];
};

export type SetupStatus = {
  initialized: boolean;
  bootstrap: string;
};

export type AuthResponse = {
  actor: Actor;
  expires_at: string;
  csrf_token?: string;
  token?: string;
};

export type BlockNode = {
  name: string;
  display_name?: string;
  path?: string;
  type: string;
  filesystem?: string;
  size_bytes?: number;
  free_bytes?: number;
  free_known?: boolean;
  unallocated_bytes?: number;
  partition_table?: string;
  mountpoints: string[];
  parent_name?: string;
  label?: string;
  uuid?: string;
  model?: string;
  vendor?: string;
  serial?: string;
  transport?: string;
  health?: string;
  temperature_c?: number;
  power_on_hours?: number;
  life_remaining_percent?: number;
  smart_available: boolean;
  smart_error?: string;
  lvm_vg_name?: string;
  lvm_lv_name?: string;
  lvm_active?: boolean;
  lvm_vg_size_bytes?: number;
  lvm_vg_free_bytes?: number;
  lvm_data_percent?: number;
  lvm_metadata_percent?: number;
  rotational: boolean;
  removable: boolean;
  system: boolean;
  children: BlockNode[];
};

export type SystemResponse = {
  version: string;
  schema_version: number;
  system: {
    node_id: string;
    hostname: string;
    os: string;
    kernel?: string;
    architecture: string;
    cpu: {
      model?: string;
      logical_cpus: number;
      usage_percent: number;
    };
    memory: {
      total_bytes?: number;
      available_bytes?: number;
    };
    uptime_seconds?: number;
    block_tree: BlockNode[];
    network_interfaces: Array<{
      name: string;
      index: number;
      mac?: string;
      mtu: number;
      up: boolean;
      loopback: boolean;
      multicast: boolean;
      oper_state?: string;
      speed_bps?: number;
      addresses: string[];
    }>;
    gpus: Array<{
      card?: string;
      vendor?: string;
      model?: string;
      vendor_id?: string;
      device_id?: string;
      class?: string;
      driver?: string;
      modalias?: string;
      pci_address?: string;
      utilization_percent?: number;
    }>;
  };
};

export type RegisteredModule = {
  manifest: {
    schema_version: number;
    id: string;
    name: string;
    description?: string;
    version: string;
    core: string;
    permissions?: string[];
    capabilities?: {
      requires?: string[];
      provides?: string[];
    };
    ui?: {
      navigation?: Array<{
        id: string;
        title: string;
        route: string;
        icon?: string;
        order?: number;
      }>;
    };
    lifecycle: string[];
  };
  status: "registered" | "enabled" | "disabled" | "error";
  error?: string;
};

export type Job = {
  id: string;
  node_id: string;
  type: string;
  status: string;
  progress: number;
  message?: string;
  error_message?: string;
  actor_type?: string;
  actor_id?: string;
  request_id?: string;
  correlation_id?: string;
  created_at: string;
};

export type AuditEntry = {
  id: string;
  occurred_at: string;
  actor_type: string;
  actor_id?: string;
  action: string;
  target_type?: string;
  target_id?: string;
  request_id?: string;
  correlation_id?: string;
  outcome: string;
  metadata: Record<string, unknown>;
};

export type RealtimeEvent = {
  version: number;
  id: string;
  stream_id: string;
  sequence: number;
  cursor?: number;
  type: string;
  time: string;
  source: {
    node_id: string;
    component: string;
  };
  request_id?: string;
  data?: unknown;
};

export type UpdateStatus = {
  current_version: string;
  available_version?: string;
  available: boolean;
  architecture: string;
  bundle_file?: string;
  bundle_size_bytes?: number;
  published_at?: string;
  notes?: string;
  helper_version?: string;
  helper_protocol?: number;
  helper_available: boolean;
  helper_compatible: boolean;
  helper_error?: string;
  rollback_available: boolean;
  rollback_version?: string;
};

export type AIStatus = {
  module_id: string;
  state: string;
  version: string;
  tool_count: number;
  provider_configured: boolean;
  provider_id?: string;
  provider_model?: string;
  provider_modes?: Array<"local" | "cloud" | "auto">;
  cloud_provider_configured: boolean;
  cloud_provider_enabled: boolean;
  cloud_provider_model?: string;
  conversation_store_ready: boolean;
};

export type ModuleNavigationState = {
  module_id: string;
  status: "registered" | "enabled" | "disabled" | "error";
  items?: Array<{
    id: string;
    title: string;
    route: string;
    icon?: string;
    order?: number;
  }>;
};

export type AIConversation = {
  id: string;
  user_id: string;
  title: string;
  created_at: string;
  updated_at: string;
  closed_at?: string;
};

export type AIAction = {
  id: string;
  conversation_id: string;
  user_id: string;
  tool_id: string;
  tool_name: string;
  sensitivity: "change" | "sensitive";
  input: Record<string, unknown>;
  status: "pending" | "executing" | "executed" | "rejected" | "failed";
  result?: unknown;
  error_code?: string;
  created_at: string;
  updated_at: string;
};

export type AIMessage = {
  id: string;
  conversation_id: string;
  role: "user" | "assistant";
  content: string;
  created_at: string;
};

export type UpdaterState = {
  phase: "idle" | "checking" | "available" | "downloading" | "ready" | "installing" | "rolling_back" | "restarting" | "succeeded" | "failed";
  current_version?: string;
  available_version?: string;
  progress_percent?: number;
  message?: string;
  error?: string;
  updated_at: string;
  published_at?: string;
  bundle_size_bytes?: number;
};


export type WireGuardPeer = {
  public_key: string;
  endpoint?: string;
  allowed_ips?: string[];
  latest_handshake?: number;
  transfer_rx?: number;
  transfer_tx?: number;
  keepalive?: number;
};

export type WireGuardTunnel = {
  name: string;
  active: boolean;
  address?: string;
  public_key?: string;
  listen_port?: number;
  peers?: WireGuardPeer[];
};

export type WireGuardStatus = {
  available: boolean;
  error?: string;
  tunnels: WireGuardTunnel[];
};

export type NetworkProfile = {
  interface: string;
  backend: string;
  supported: boolean;
  managed: boolean;
  ownership?: "none" | "home-ai" | "external" | "conflict";
  method?: string;
  address?: string;
  gateway?: string;
  dns?: string[];
  source?: string;
  error?: string;
};

export type NetworkProfileStatus = {
  backend: string;
  profiles: NetworkProfile[];
};

export type StoragePurpose = "files" | "video";

export type StoragePurposeUsage = {
  type: "file_pool";
  id: string;
  name: string;
  root_path?: string;
};

export type StoragePurposeAssignment = {
  device: string;
  filesystem_uuid?: string;
  purpose: StoragePurpose;
  present: boolean;
  filesystem?: string;
  label?: string;
  mountpoints?: string[] | null;
  size_bytes?: number;
  free_bytes?: number;
  free_known: boolean;
  in_use: boolean;
  used_by: StoragePurposeUsage[];
};

export type FilePool = {
  id: string;
  name: string;
  root_path: string;
  storage_device?: string;
  storage_filesystem_uuid?: string;
  reserve_percent: number;
  warning_percent: number;
  capacity_known: boolean;
  size_bytes?: number;
  free_bytes?: number;
  reserve_bytes?: number;
  warning_bytes?: number;
  capacity_state: "unknown" | "ok" | "warning" | "reserve";
};

export type FileFolder = {
  quota_bytes: number;
  hard_quota_bytes: number;
  used_bytes: number;
  reserved_bytes: number;
  usage_known: boolean;
  id: string;
  pool_id: string;
  pool_name: string;
  name: string;
  kind: "private" | "shared";
  owner_user_id?: string;
  relative_path: string;
  can_read: boolean;
  can_write: boolean;
};

export type FileEntry = {
  name: string;
  path: string;
  kind: "file" | "directory" | "symlink";
  size_bytes?: number;
  modified_at: string;
};

export type FileTrashEntry = {
  id: string;
  original_path: string;
  name: string;
  kind: "file" | "directory";
  size_bytes?: number;
  deleted_at: string;
};

export type FileUploadChunk = {
  offset: number;
  size: number;
  sha256: string;
};

export type FileUploadSession = {
  id: string;
  path: string;
  total_bytes: number;
  received_bytes: number;
  expected_sha256?: string;
  client_fingerprint?: string;
  chunks?: FileUploadChunk[];
  created_at: string;
  updated_at: string;
};

export type FileUploadResult = {
  path: string;
  size_bytes: number;
  sha256: string;
};

export type SMBUser = {
  user_id: string;
  username: string;
  display_name: string;
  smb_username: string;
  configured: boolean;
};

export type SMBShare = {
  writable: boolean;
  write_restriction?: string;
  folder_id: string;
  folder_name: string;
  kind: "private" | "shared";
  share_name: string;
  unc: string;
};

export type SMBStatus = {
  available: boolean;
  active: boolean;
  error?: string;
  hostname: string;
  workgroup: string;
  users: SMBUser[];
  shares: SMBShare[];
  hard_quota_ready: boolean;
  hard_quota_error?: string;
};



export type NVRStatus = {
  module_id: string;
  state: "registered" | "enabled" | "disabled" | "error";
  version: string;
  camera_count: number;
  online_count: number;
  offline_count: number;
  supervisor_running: boolean;
  media_runtime_ready: boolean;
  secret_store_ready: boolean;
  foundation_stage: string;
};

export type NVRCamera = {
  id: string;
  name: string;
  enabled: boolean;
  source_type: "rtsp" | "onvif";
  transport: "tcp" | "udp";
  recording_mode: "off" | "continuous" | "motion";
  audio_enabled: boolean;
  has_credentials: boolean;
  runtime: {
    state: "disabled" | "connecting" | "online" | "offline";
    last_seen_at?: string;
    last_checked_at?: string;
    last_error?: string;
    reconnect_count: number;
  };
  created_at: string;
  updated_at: string;
};

export type NVRStreamProfile = {
  id: string;
  camera_id: string;
  role: "main" | "sub";
  codec?: string;
  width?: number;
  height?: number;
  fps?: number;
  bitrate_bps?: number;
  created_at: string;
  updated_at: string;
};

export type NVRCameraConfig = NVRCamera & {
  address: string;
  profiles?: NVRStreamProfile[];
};

export type NVRProbe = {
  codec: string;
  width: number;
  height: number;
  fps: number;
  bitrate_bps: number;
  has_audio: boolean;
};

export type NVRCameraInput = {
  name: string;
  address: string;
  username?: string;
  password?: string;
  transport?: "tcp" | "udp";
  recording_mode?: "off" | "continuous" | "motion";
  audio_enabled: boolean;
  enabled?: boolean;
  clear_credentials?: boolean;
};
