import { describe, expect, it } from "vitest";
import type { StoragePurposeAssignment } from "../api/types";
import { filePoolMountStatus } from "./filePoolMount";

const root = "/mnt/home-ai-core/sdb1";
const pool = {
  root_path: root, storage_device: "/dev/sdb1", storage_filesystem_uuid: "fs-abc",
};
const base: StoragePurposeAssignment = {
  device: "/dev/sdb1", filesystem_uuid: "fs-abc", purpose: "files",
  present: true, filesystem: "ext4", mountpoints: [root],
  free_known: false, in_use: true, used_by: [],
};

describe("NAS pool mount-status advisory", () => {
  it("recognizes a verified matching mount path", () => {
    expect(filePoolMountStatus(pool, [base])).toBe("mounted");
  });
  it("requires the exact saved pool path, never a parent or child", () => {
    expect(filePoolMountStatus(pool, [{...base, mountpoints: ["/mnt/home-ai-core"]}])).toBe("unmounted");
    expect(filePoolMountStatus(pool, [{...base, mountpoints: [root + "/files"]}])).toBe("unmounted");
    expect(filePoolMountStatus(pool, [{...base, mountpoints: []}])).toBe("unmounted");
  });
  it("marks absent devices separately and unknown inventory as unknown", () => {
    expect(filePoolMountStatus(pool, [{...base, present: false, mountpoints: []}])).toBe("missing");
    expect(filePoolMountStatus(pool, [])).toBe("unknown");
    expect(filePoolMountStatus(pool, [{...base, purpose: "nvr"}])).toBe("unknown");
  });
  it("uses filesystem UUID when the device name changes", () => {
    expect(filePoolMountStatus(pool, [{...base, device: "/dev/sdc1"}])).toBe("mounted");
    expect(filePoolMountStatus(pool, [{...base, filesystem_uuid: "fs-other"}])).toBe("unknown");
  });
});
