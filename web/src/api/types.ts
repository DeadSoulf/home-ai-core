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

export type UserAccount = {
  id: string;
  username: string;
  display_name: string;
  disabled: boolean;
  created_at: string;
  last_login_at?: string;
  roles: string[];
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

export type FilePool = {
  id: string;
  name: string;
  root_path: string;
};

export type FileFolder = {
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

