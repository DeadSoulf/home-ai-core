export type Actor = {
  type: string;
  id: string;
  username?: string;
  display_name?: string;
  roles: string[];
  permissions: string[];
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
    block_devices: Array<{
      name: string;
      display_name?: string;
      path: string;
      major_minor?: string;
      vendor?: string;
      model?: string;
      serial?: string;
      size_bytes?: number;
      rotational: boolean;
      removable: boolean;
      system: boolean;
      partitions: Array<{
        name: string;
        path: string;
        size_bytes?: number;
        filesystem?: string;
        uuid?: string;
        label?: string;
        mountpoints: string[];
      }>;
    }>;
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
