import { describe, expect, it } from "vitest";
import { chooseInitialLocale } from "./i18n";

describe("language selection", () => {
  it("uses stored preference first", () => {
    expect(chooseInitialLocale("en", ["ru-RU"])).toBe("en");
  });

  it("defaults Russian browsers to Russian", () => {
    expect(chooseInitialLocale(null, ["de-DE", "ru-RU"])).toBe("ru");
  });

  it("falls back to English", () => {
    expect(chooseInitialLocale(null, ["de-DE"])).toBe("en");
  });
});
