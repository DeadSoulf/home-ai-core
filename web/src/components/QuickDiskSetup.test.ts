import { describe, expect, it } from "vitest";
import type { BlockNode } from "../api/types";
import { quickDiskSetup } from "./QuickDiskSetup";

function disk(overrides: Partial<BlockNode> = {}): BlockNode {
  return {
    name: "sdb",
    path: "/dev/sdb",
    type: "disk",
    filesystem: "",
    partition_table: "",
    size_bytes: 1000 * 1024 ** 3,
    unallocated_bytes: 1000 * 1024 ** 3,
    mountpoints: [],
    children: [],
    system: false,
    removable: false,
    rotational: true,
    smart_available: true,
    ...overrides,
  };
}
function partition(overrides: Partial<BlockNode> = {}): BlockNode {
  return {
    name: "sdb1",
    path: "/dev/sdb1",
    type: "part",
    filesystem: "ext4",
    mountpoints: [],
    children: [],
    system: false,
    removable: false,
    rotational: true,
    smart_available: false,
    ...overrides,
  };
}

describe("quick disk setup eligibility", () => {
  it("offers full setup only for an unpartitioned unformatted data disk", () => {
    expect(quickDiskSetup(disk()).kind).toBe("blank");
  });
  it("rejects the system disk even if the inventory is empty", () => {
    expect(quickDiskSetup(disk({system: true})).kind).toBe("unavailable");
  });
  it("rejects an old partition table or raw filesystem", () => {
    expect(quickDiskSetup(disk({partition_table: "gpt"})).kind).toBe("unavailable");
    expect(quickDiskSetup(disk({filesystem: "xfs"})).kind).toBe("unavailable");
  });
  it("rejects mounted, failed and incompletely inventoried disks", () => {
    expect(quickDiskSetup(disk({mountpoints: ["/data"]})).kind).toBe("unavailable");
    expect(quickDiskSetup(disk({health: "failed"})).kind).toBe("unavailable");
    expect(quickDiskSetup(disk({unallocated_bytes: undefined})).kind).toBe("unavailable");
  });
  it("offers non-destructive connection only for one eligible filesystem", () => {
    expect(quickDiskSetup(disk({children: [partition()], partition_table: "gpt"})).kind).toBe("existing");
    expect(quickDiskSetup(disk({children: [partition({filesystem: "xfs"})]})).kind).toBe("existing");
    expect(quickDiskSetup(disk({children: [partition({filesystem: "swap"})]})).kind).toBe("unavailable");
    expect(quickDiskSetup(disk({children: [partition({mountpoints: ["/data"]})]})).kind).toBe("unavailable");
    expect(quickDiskSetup(disk({children: [partition(), partition({name: "sdb2"})]})).kind).toBe("unavailable");
  });
});
