import { describe, expect, it } from "vitest";
import { APIError } from "../api/client";
import { describeFileError } from "./fileErrors";
import { uiMessages } from "./messages";

describe("Files API failure reporting", () => {
  it("explains folder storage HTTP 502 without claiming Core is offline", () => {
    const failure = new APIError(502, {error: {
      code: "file_folder_storage_unavailable",
      message: "inspect logical folder root: permission denied",
      request_id: "req-test-storage",
    }});
    const summary = describeFileError(failure, "ru", "fallback");
    expect(summary).toContain("Хранилище папки недоступно");
    expect(summary).not.toBe("Сервер недоступен");
    expect(failure.rawMessage).toContain("permission denied");
    expect(failure.requestId).toBe("req-test-storage");
  });
  it("explains a NAS pool mountpoint mismatch, keeping technical details available", () => {
    for (const rawMessage of [
      "NAS pool root must be the filesystem mount point itself",
      "NAS pool root /mnt/home-ai-core/disk1 must be the filesystem mount point itself (current filesystem mount covering it: /)",
      "NAS pool mount /mnt/home-ai-core/disk1 is present on the host but unavailable in the updater helper mount namespace",
    ]) {
      const error = new APIError(502, {error: {
        code: "file_folder_prepare_failed", message: rawMessage,
        request_id: "aca5e088eed8f01c96ed25e6",
      }});
      expect(describeFileError(error, "ru", "fallback")).toContain("Файлы → Хранилище");
      expect(describeFileError(error, "en", "fallback")).toContain("Files → Storage");
      expect(error.requestId).toBe("aca5e088eed8f01c96ed25e6");
    }
  });
  it("handles unrecognized HTTP 503 as a received server response", () => {
    const failure = new APIError(503, {error: {code: "unknown_file_error", message: "service unavailable"}});
    expect(describeFileError(failure, "ru", "fallback")).toContain("HTTP 503");
    expect(describeFileError(failure, "en", "fallback")).toContain("HTTP 503");
    expect(describeFileError(failure, "ru", "fallback")).not.toBe("Сервер недоступен");
  });
  it("separates SMB, quotas and database errors from network disconnects", () => {
    for (const [code, part] of [
      ["smb_access_suspend_failed", "SMB"],
      ["file_usage_unavailable", "квоты"],
      ["file_management_unavailable", "Недоступны данные"],
      ["file_trash_unavailable", "Корзина папки"],
      ["file_pool_storage_not_assigned", "не подготовлен"],
    ]) {
      const error = new APIError(502, {error: {code, message: "test backend failure"}});
      expect(describeFileError(error, "ru", "fallback")).toContain(part);
    }
  });
  it("continues to distinguish connection loss and ordinary errors", () => {
    expect(describeFileError(new TypeError("Failed to fetch"), "ru", "fallback")).toContain("Нет связи");
    expect(describeFileError(new Error("local validation"), "ru", "fallback")).toBe("local validation");
    expect(describeFileError(undefined, "ru", "fallback")).toBe("fallback");
  });
  it("keeps matching RU and EN message dictionaries", () => {
    expect(Object.keys(uiMessages.en).sort()).toEqual(Object.keys(uiMessages.ru).sort());
  });
});
