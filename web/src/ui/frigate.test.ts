import {describe, expect, it} from "vitest";
import {frigateUIURL} from "./frigate";

describe("original Frigate origin isolation", () => {
  it("uses the gateway advertised by Core without assuming its port", () => {
    expect(frigateUIURL("https://home.test:82/modules/frigate.module", "https://home.test:18971/")).toBe("https://home.test:18971/");
    expect(frigateUIURL("https://[2001:db8::1]:82/", "https://[2001:db8::1]:8971/")).toBe("https://[2001:db8::1]:8971/");
  });
  it("rejects arbitrary hosts, Core origin, plaintext and URL credentials", () => {
    for (const target of ["https://attacker.test:8971/", "https://home.test:82/", "http://home.test:8971/", "https://user:secret@home.test:8971/", "https://home.test:8971/?token=secret", "https://home.test:8971/#token", "https://home.test:8971/api", "javascript:alert(1)"]) {
      expect(() => frigateUIURL("https://home.test:82/", target)).toThrow();
    }
  });
  it("permits explicitly configured literal-loopback development only", () => {
    expect(frigateUIURL("http://127.0.0.1:18082/", "http://127.0.0.1:18971/")).toBe("http://127.0.0.1:18971/");
    expect(() => frigateUIURL("http://192.168.1.10:82/", "http://192.168.1.10:8971/")).toThrow();
  });
});
