import type { FilePool, StoragePurposeAssignment } from "../api/types";

export type FilePoolMountStatus = "mounted" | "unmounted" | "missing" | "unknown";

/**
 * Read-only advisory from the current block-device inventory. The backend
 * privileged mountpoint validation is authoritative, not this UI heuristic.
 * Missing inventory must never be treated as proof that a mount is healthy.
 */
export function filePoolMountStatus(
  pool: Pick<FilePool, "root_path" | "storage_device" | "storage_filesystem_uuid">,
  assignments: readonly StoragePurposeAssignment[],
): FilePoolMountStatus {
  const matching = assignments.filter((entry) => {
    if (entry.purpose !== "files") return false;
    // A filesystem UUID survives a renamed /dev/sdX device.
    if (pool.storage_filesystem_uuid && entry.filesystem_uuid) {
      return pool.storage_filesystem_uuid === entry.filesystem_uuid;
    }
    return !!pool.storage_device && entry.device === pool.storage_device;
  });
  if (matching.length === 0) return "unknown";
  if (matching.some((entry) =>
    entry.present && entry.filesystem && (entry.mountpoints || []).includes(pool.root_path))) {
    return "mounted";
  }
  if (matching.every((entry) => !entry.present)) return "missing";
  return "unmounted";
}
