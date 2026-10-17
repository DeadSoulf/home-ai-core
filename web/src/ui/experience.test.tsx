import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { SectionGuide } from "../components/SectionGuide";
import { NavigationIcon } from "../components/NavigationIcon";
import { filesGuides, navigationHint, quickDestinations, systemGuides, visibleQuickDestinations } from "./experience";
import { uiMessages } from "./messages";

describe("guided interface and permissions", () => {
  it("never displays shortcuts to sections the user cannot open", () => {
    expect(visibleQuickDestinations([])).toEqual([]);
    expect(visibleQuickDestinations(["security.self.read"]).map((link) => link.path)).toEqual(["/files"]);
    expect(visibleQuickDestinations(["system.read"]).map((link) => link.path)).toEqual(["/system#storage"]);
    expect(visibleQuickDestinations(["updates.read"])).toEqual([]);
    expect(visibleQuickDestinations(["network.read"])).toEqual([]);
    expect(visibleQuickDestinations(["jobs.read", "audit.read"]).map((link) => link.path)).toEqual(["/jobs"]);
    expect(visibleQuickDestinations(["security.users.read"]).map((link) => link.path)).toEqual(["/users"]);
  });

  it("provides a stable unique destination for every quick task", () => {
    expect(new Set(quickDestinations.map((link) => link.path)).size).toBe(quickDestinations.length);
    const allPermissions = Array.from(new Set(quickDestinations.flatMap((link) => link.permissions)));
    expect(visibleQuickDestinations(allPermissions).map((link) => link.path)).toEqual(quickDestinations.map((link) => link.path));
  });

  it("localizes every new navigation and contextual-help label", () => {
    const keys = [
      ...quickDestinations.flatMap((item) => [item.title, item.description]),
      ...Object.values(filesGuides).flatMap((item) => [item.title, item.description]),
      ...Object.values(systemGuides).flatMap((item) => [item.title, item.description]),
      ...["/", "/files", "/modules", "/system", "/users", "/jobs", "/audit", "/modules/custom"].map(navigationHint),
    ];
    for (const key of keys) {
      expect(uiMessages.en[key], "missing English " + key).toBeTruthy();
      expect(uiMessages.ru[key], "missing Russian " + key).toBeTruthy();
    }
  });

  it("renders a readable help section without interactive side effects", () => {
    const html = renderToStaticMarkup(<SectionGuide title="Files" description="Choose a folder to open." />);
    expect(html).toContain("Files");
    expect(html).toContain("Choose a folder to open.");
    expect(html).toContain("ux-section-guide");
  });

  it("uses a pictogram next to textual navigation labels", () => {
    const html = renderToStaticMarkup(<NavigationIcon path="/system#storage" />);
    expect(html).toContain("<svg");
    expect(html).toContain('aria-hidden="true"');
    expect(html).toContain("<rect");
  });

  it("keeps translation dictionaries aligned", () => {
    expect(Object.keys(uiMessages.en).sort()).toEqual(Object.keys(uiMessages.ru).sort());
  });
});
