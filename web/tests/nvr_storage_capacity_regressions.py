"""Test the shipped NVR page with a mounted camera disk and isolated APIs.

No real disk, NVR camera, production host or Docker container is touched.
"""
import asyncio
import copy
import json

from playwright.async_api import async_playwright, expect
import browser_regressions as ui


async def main():
    video_disk = {
        "disks": [{
            "device": "/dev/sdb1", "label": "Видеоархив", "filesystem": "ext4",
            "mountpoint": "/mnt/home-ai-core/sdb1", "state": "mounted",
            "capacity_known": True,
            "total_bytes": 1024**4, "free_bytes": 700*1024**3,
            "used_bytes": 324*1024**3,
        }],
        "assigned_count": 1, "mounted_count": 1, "capacity_known": True,
        "total_bytes": 1024**4, "free_bytes": 700*1024**3,
    }

    async def respond_json(route, value):
        await route.fulfill(status=200, content_type="application/json", body=json.dumps(value))

    async def storage_endpoint(route):
        await respond_json(route, video_disk)

    overrides = {
        "/api/v1/nvr/status": lambda route: respond_json(route, {
            "ok": True, "service": "nvr", "cameras": 1, "max_cameras": 32,
            "active_recorders": 1, "recordings": 4, "storage_bytes": 100*1024**3,
            "retention_days": 7, "max_storage_gb": 500,
        }),
        "/api/v1/nvr/cameras": lambda route: respond_json(route, {"ok": True, "cameras": []}),
        "/api/v1/nvr/recordings": lambda route: respond_json(route, {"ok": True, "recordings": []}),
        "/api/v1/nvr/storage": storage_endpoint,
    }
    module = copy.deepcopy(ui.MANIFEST)
    module["id"] = "nvr.module"
    module["name"] = "NVR"
    module["ui"] = {"navigation": [{
        "id": "nvr", "title": "NVR", "route": "/modules/nvr.module", "icon": "video"
    }]}
    installed = [{"manifest": module, "status": "enabled"}]

    async with async_playwright() as playwright:
        browser = await playwright.chromium.launch(headless=True, args=["--no-sandbox"])
        harness = await ui.Harness(browser, modules=installed, overrides=overrides).start("/modules/nvr.module")
        try:
            panel = harness.page.locator(".nvr-page").get_by_role("region", name="Хранилище NVR")
            # A section with a title is not automatically an aria named region.
            panel = harness.page.locator(".nvr-page .nvr-storage-device-list")
            await expect(panel).to_be_visible()
            await expect(harness.page.locator(".nvr-storage-summary")).to_contain_text("700 ГиБ")
            await expect(harness.page.locator(".nvr-storage-summary")).to_contain_text("1 ТиБ")
            await expect(panel).to_contain_text("Видеоархив")
            await expect(panel).to_contain_text("/mnt/home-ai-core/sdb1")
            await expect(harness.page.locator(".nvr-storage-progress")).to_be_visible()
            await expect(harness.page.locator(".nvr-stat-grid")).to_contain_text("Размер видеоархива")
            await expect(harness.page.locator(".nvr-storage-warning")).to_contain_text(
                "не подтверждает, что NVR записывает видео"
            )

            video_disk.update(disks=[{
                "device": "/dev/sdb1", "label": "Видеоархив",
                "state": "unmounted", "size_bytes": 1024**4,
                "capacity_known": False,
            }], mounted_count=0, capacity_known=False, total_bytes=0, free_bytes=0)
            await harness.page.get_by_role("button", name="Обновить", exact=True).click()
            await expect(panel).to_contain_text("Не смонтирован")
            await expect(harness.page.locator(".nvr-storage-summary")).to_contain_text("—")
            assert not harness.requests, "NVR capacity must not mutate storage"
            await harness.close()
        finally:
            await browser.close()

    print("PASS: NVR reports mounted video disk space and unmounted unknown state without changing disks.")


if __name__ == "__main__":
    asyncio.run(main())
