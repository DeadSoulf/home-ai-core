import { describe, expect, it } from "vitest";
import { homeAssistantURL } from "./homeAssistant";

describe("Home Assistant independent origin", () => {
  it("uses the same host and port 8123 without Core path or tokens", () => {
    expect(homeAssistantURL("https://core.local:8443/modules?token=secret#private")).toBe("http://core.local:8123/");
  });
  it("keeps IPv6 addresses valid", () => {
    expect(homeAssistantURL("https://[2001:db8::1]:8082/modules")).toBe("http://[2001:db8::1]:8123/");
  });
  it("does not forward URL credentials", () => {
    expect(homeAssistantURL("https://user:secret@core.local/modules")).toBe("http://core.local:8123/");
  });
  it("rejects non-web origins", () => {
    expect(() => homeAssistantURL("javascript:alert(1)")).toThrow();
  });
});
