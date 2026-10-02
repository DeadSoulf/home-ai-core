import { describe, expect, it } from "vitest";
import { keepAIPageMounted } from "./App";

describe("AI page navigation lifecycle", () => {
  it("mounts the AI page when /ai is opened", () => {
    expect(keepAIPageMounted(false, "/ai")).toBe(true);
  });

  it("keeps the AI page mounted after navigating away", () => {
    expect(keepAIPageMounted(true, "/system")).toBe(true);
    expect(keepAIPageMounted(true, "/files/storage")).toBe(true);
  });

  it("does not mount AI before the first visit", () => {
    expect(keepAIPageMounted(false, "/system")).toBe(false);
  });
});
