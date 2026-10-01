import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BlockNode } from "../api/types";
import { I18nProvider } from "../i18n";
import { StorageDevices } from "./StorageDevices";

function partition(overrides: Partial<BlockNode> = {}): BlockNode {
  return {
    name: "sda1",
    path: "/dev/sda1",
    type: "part",
    filesystem: "ext4",
    size_bytes: 500 * 1024 ** 3,
    free_bytes: 300 * 1024 ** 3,
    free_known: true,
    mountpoints: ["/mnt/home-ai-core/data"],
    smart_available: false,
    rotational: false,
    removable: false,
    system: false,
    children: [],
    ...overrides,
  };
}

function disk(overrides: Partial<BlockNode> = {}): BlockNode {
  return {
    name: "sda",
    display_name: "Data disk",
    path: "/dev/sda",
    type: "disk",
    size_bytes: 1000 * 1024 ** 3,
    unallocated_bytes: 500 * 1024 ** 3,
    mountpoints: [],
    model: "Test SSD",
    transport: "sata",
    smart_available: true,
    health: "ok",
    rotational: false,
    removable: false,
    system: false,
    children: [partition({label: "Files partition"})],
    ...overrides,
  };
}

describe("storage disk-first overview", () => {
  beforeEach(() => {
    vi.stubGlobal("navigator", {languages: ["ru-RU"]});
    vi.stubGlobal("localStorage", {
      getItem: () => "ru",
      setItem: vi.fn(),
      removeItem: vi.fn(),
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  it("shows only physical disks before a disk is selected", () => {
    const markup = renderToStaticMarkup(
      <I18nProvider>
        <StorageDevices
          devices={[
            disk(),
            disk({
              name: "sdb",
              display_name: "Video disk",
              path: "/dev/sdb",
              children: [partition({name: "sdb1", path: "/dev/sdb1", label: "Video partition"})],
            }),
          ]}
          canManage
          onChanged={() => undefined}
        />
      </I18nProvider>,
    );

    expect(markup.match(/class="storage-disk-card/g)).toHaveLength(2);
    expect(markup).toContain("Data disk");
    expect(markup).toContain("Video disk");
    expect(markup).not.toContain("Files partition");
    expect(markup).not.toContain("Video partition");
    expect(markup).not.toContain("Создать раздел");
    expect(markup).not.toContain("Форматировать");
    expect(markup).not.toContain("storage-drawer");
  });

  it("keeps the system-disk warning visible in the disk overview", () => {
    const markup = renderToStaticMarkup(
      <I18nProvider>
        <StorageDevices
          devices={[disk({system: true, display_name: "System disk"})]}
          canManage
          onChanged={() => undefined}
        />
      </I18nProvider>,
    );

    expect(markup).toContain("Системный диск");
    expect(markup).toContain("storage-protection");
  });
});
