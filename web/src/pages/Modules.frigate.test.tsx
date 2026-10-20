import {beforeEach, describe, expect, it, vi} from "vitest";
import {renderToStaticMarkup} from "react-dom/server";
import type {RegisteredModule} from "../api/types";
import {ModulesPage} from "./Modules";

const resources = vi.hoisted(() => ({items: [] as unknown[]}));
vi.mock("../hooks/useResource", () => ({useResource: () => ({data: resources.items.shift(), loading: false, error: undefined, reload: () => {}})}));
vi.mock("../i18n", () => ({useI18n: () => ({t: (key: string) => key, date: (value: string) => value, status: (value: string) => value})}));
vi.mock("../ui/useUI", () => ({useUI: () => (key: string) => key}));

const manifest: RegisteredModule["manifest"] = {
  schema_version: 2, id: "frigate.module", name: "Frigate NVR", version: "0.18.2", core: ">=0.1.211 <1.0.0",
  runtime: {type: "docker", docker: {image: `ghcr.io/blakeblackshear/frigate@sha256:${"a".repeat(64)}`}},
  lifecycle: ["install", "upgrade", "remove", "start", "stop", "restart"],
};
function setup(installed: RegisteredModule[] = [], stale = false) {
  resources.items = [
    {modules: installed, capabilities: []},
    {id: "official", modules: [manifest], frigate_upstream: {stale, repository: "blakeblackshear/frigate", supported_range: ">=0.18.2 <0.19.0"}},
    {network_ready: true}, [], undefined,
  ];
}
beforeEach(() => setup());
describe("Frigate module listing safety", () => {
  it("keeps only one global update check, not per-card copies", () => {
    setup([{manifest, status: "enabled"}]);
    const html = renderToStaticMarkup(<ModulesPage revision={0} canManage section="installed" />);
    expect((html.match(/>moduleUpgradeCheck<\/button>/g) || []).length).toBe(1);
    expect(html).not.toContain("data-module-upgrade=");
    expect(html).toContain("frigate.module");
  });
  it("disables installation while official Frigate data is stale", () => {
    setup([], true);
    const html = renderToStaticMarkup(<ModulesPage revision={0} canManage section="catalog" />);
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>moduleInstall<\/button>/);
  });
  it("does not offer reinstall for an already installed module", () => {
    setup([{manifest, status: "enabled"}]);
    const html = renderToStaticMarkup(<ModulesPage revision={0} canManage section="catalog" />);
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>moduleInstalled<\/button>/);
  });
  it("enables a freshly verified, uninstalled module for manual approval", () => {
    setup([], false);
    const html = renderToStaticMarkup(<ModulesPage revision={0} canManage section="catalog" />);
    expect(html).toContain('class="button primary">moduleInstall</button>');
  });
});
