import {renderToStaticMarkup} from "react-dom/server";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
import type {BlockNode} from "../api/types";
import {I18nProvider} from "../i18n";
import {StorageDevices} from "./StorageDevices";
import {StoragePurposeOptions, supportedStoragePurposes} from "./StoragePurposeOptions";

const partition: BlockNode = {
  name: "sdb1", path: "/dev/sdb1", type: "part", filesystem: "ext4",
  size_bytes: 1024 * 1024 * 1024, free_bytes: 0, mountpoints: [],
  smart_available: false, rotational: true, removable: false, system: false, children: [],
};
const disk: BlockNode = {
  name: "sdb", path: "/dev/sdb", type: "disk", size_bytes: 8 * 1024 ** 3,
  unallocated_bytes: 3 * 1024 ** 3, mountpoints: [], smart_available: false,
  rotational: true, removable: false, system: false, children: [partition],
};

function render(child: React.ReactNode) {
  return renderToStaticMarkup(<I18nProvider>{child}</I18nProvider>);
}

describe("partition owner/module purpose selection", () => {
  beforeEach(() => {
    vi.stubGlobal("navigator", {languages: ["ru-RU"]});
    vi.stubGlobal("localStorage", {getItem: (key: string) =>
      key === "home-ai-core.locale" ? "ru" : null});
  });
  afterEach(() => vi.unstubAllGlobals());

  it("exposes only backend-supported destinations including NVR", () => {
    expect(supportedStoragePurposes).toEqual(["files", "nvr"]);
    const html = render(<select defaultValue="nvr"><StoragePurposeOptions includeNone /></select>);
    expect(html).toContain('value="files"');
    expect(html).toContain('value="nvr"');
    expect(html).toContain("NVR · Видеоархив");
    expect(html).toContain('value=""');
  });

  it("lets an existing partition or logical volume be assigned to NVR in tree view", () => {
    const html = render(<StorageDevices devices={[disk]} canManage onChanged={() => {}} mode="tree" expandedByDefault />);
    expect(html).toContain('value="nvr"');
    expect(html).toContain("Для какого модуля");
  });

  it("keeps an NVR destination in the disk details partition editor", () => {
    const html = render(<StorageDevices devices={[disk]} canManage onChanged={() => {}} mode="partitions" expandedByDefault />);
    expect(html).toContain('value="nvr"');
    expect(html).toContain("NVR · Видеоархив");
  });
});
