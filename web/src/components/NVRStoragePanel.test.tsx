import {renderToStaticMarkup} from "react-dom/server";
import {describe, expect, it} from "vitest";
import type {NVRStorageStatus} from "../api/types";
import {formatNVRBytes, NVRStoragePanel} from "./NVRStoragePanel";

const mounted: NVRStorageStatus = {
  disks: [{
    device: "/dev/sdb1", label: "Архив камер", filesystem: "ext4",
    mountpoint: "/mnt/home-ai-core/sdb1", state: "mounted",
    capacity_known: true, total_bytes: 1024 ** 4,
    free_bytes: 700 * 1024 ** 3, used_bytes: 324 * 1024 ** 3,
  }],
  assigned_count: 1, mounted_count: 1, capacity_known: true,
  total_bytes: 1024 ** 4, free_bytes: 700 * 1024 ** 3,
};

describe("NVR physical disk capacity", () => {
  it("displays actual free, total, used and the mountpoint", () => {
    const html = renderToStaticMarkup(<NVRStoragePanel storage={mounted} loading={false} error="" />);
    expect(html).toContain("Хранилище NVR");
    expect(html).toContain("700 ГиБ");
    expect(html).toContain("1 ТиБ");
    expect(html).toContain("324 ГиБ");
    expect(html).toContain("Архив камер");
    expect(html).toContain("/mnt/home-ai-core/sdb1");
    expect(html).toContain('role="progressbar"');
    expect(html).toContain("не подтверждает, что NVR записывает видео");
  });

  it("does not render an unknown/unmounted disk as 0 GB free", () => {
    const unknown: NVRStorageStatus = {
      disks: [{device: "/dev/sdc1", state: "unmounted", capacity_known: false, size_bytes: 2 * 1024 ** 4}],
      assigned_count: 1, mounted_count: 0, capacity_known: false,
    };
    const html = renderToStaticMarkup(<NVRStoragePanel storage={unknown} loading={false} error="" />);
    expect(html).toContain("Не смонтирован");
    expect(html).toContain("Физический размер раздела");
    expect(html).toContain("—");
    expect(html).not.toContain('role="progressbar"');
    expect(html).not.toContain("0 ГиБ");
  });

  it("does not confuse archive file bytes with free filesystem capacity", () => {
    const html = renderToStaticMarkup(<NVRStoragePanel storage={{
      disks: [], assigned_count: 0, mounted_count: 0, capacity_known: false,
    }} loading={false} error="" />);
    expect(html).toContain("Диск для NVR пока не назначен");
    expect(formatNVRBytes(Number.NaN)).toBe("—");
  });
});
