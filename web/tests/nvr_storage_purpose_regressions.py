"""Verify the real production UI lets users choose NVR when creating and assigning a partition.

This uses isolated API fixtures only: no real disk, partition table, Docker
container, camera, or files are ever modified.
"""
import asyncio
import copy
import json

from playwright.async_api import async_playwright, expect
import browser_regressions as ui


async def main():
    disk = {
        "name": "sdb", "path": "/dev/sdb", "type": "disk",
        "size_bytes": 10737418240, "unallocated_bytes": 6442450944,
        "partition_table": "gpt", "mountpoints": [], "children": [],
        "system": False, "rotational": True, "removable": False,
        "smart_available": False, "health": "ok",
    }
    partition = {
        "name": "sdb1", "path": "/dev/sdb1", "type": "part",
        "size_bytes": 1073741824, "filesystem": "ext4",
        "mountpoints": [], "children": [],
        "system": False, "rotational": True, "removable": False,
        "smart_available": False,
    }
    disk["children"] = [partition]
    ui.SYSTEM = copy.deepcopy(ui.SYSTEM)
    ui.SYSTEM["system"]["block_tree"] = [disk]
    assignments = []
    posted = []

    async def storage_purposes(route):
        request = route.request
        if request.method == "GET":
            body = {"assignments": assignments}
        else:
            info = request.post_data_json
            posted.append(info)
            assignments[:] = [item for item in assignments if item["device"] != info["device"]]
            if info.get("purpose") not in (None, "none", ""):
                assignments.append({
                    "device": info["device"], "purpose": info["purpose"],
                    "present": True, "filesystem": "ext4", "mountpoints": [],
                    "free_known": False, "in_use": False, "used_by": [],
                })
            body = {"assignment": assignments[-1] if assignments else None}
        await route.fulfill(status=200, content_type="application/json", body=json.dumps(body))

    async def storage_operation(route):
        payload = route.request.post_data_json
        posted.append(payload)
        if payload.get("operation") != "partition.create":
            raise AssertionError(f"unexpected storage operation {payload}")
        if payload.get("dry_run"):
            body = {"message": "plan accepted", "dry_run": True}
        else:
            assert payload.get("purpose") == "nvr", payload
            assert payload.get("confirm") == "CREATE /dev/sdb", payload
            # Simulate the trusted helper: a new partition was created and
            # assigned to NVR without formatting or mounting it.
            created = {**partition, "name": "sdb2", "path": "/dev/sdb2", "filesystem": ""}
            disk["children"].append(created)
            disk["unallocated_bytes"] -= 1073741824
            assignments.append({
                "device": "/dev/sdb2", "purpose": "nvr", "present": True,
                "filesystem": "", "mountpoints": [], "free_known": False,
                "in_use": False, "used_by": [],
            })
            body = {
                "message": "partition created", "device": "/dev/sdb2",
                "purpose": "nvr", "purpose_assigned": True,
            }
        await route.fulfill(status=200, content_type="application/json", body=json.dumps(body))

    actor = copy.deepcopy(ui.ACTOR)
    actor["permissions"] = list(set(actor["permissions"] + ["storage.manage", "files.manage"]))
    overrides = {
        "/api/v1/storage/purposes": storage_purposes,
        "/api/v1/storage/operation": storage_operation,
    }
    async with async_playwright() as playwright:
        browser = await playwright.chromium.launch(headless=True, args=["--no-sandbox"])
        harness = await ui.Harness(browser, actor=actor, overrides=overrides).start("/system#storage")
        try:
            await expect(harness.page.get_by_text("sdb", exact=True).first).to_be_visible()
            await harness.page.locator(".storage-disk-row").first.click()
            # Existing partition: select NVR without formatting.
            existing = harness.page.locator('.storage-disk-manager .storage-purpose-control select').first
            await expect(existing.locator('option[value="nvr"]')).to_have_text("NVR · Видеоархив")
            await existing.select_option("nvr")
            await expect(existing).to_have_value("nvr")

            # New partition: the second, top-level Create Partition form must
            # show the same NVR choice instead of Files-only.
            await harness.page.locator(".storage-disk-actions-wrap button").get_by_text(
                "Создать раздел", exact=True
            ).click()
            purpose = harness.page.locator(".storage-disk-create-form select")
            await expect(purpose.locator('option[value="nvr"]')).to_have_text("NVR · Видеоархив")
            await purpose.select_option("nvr")
            await expect(harness.page.get_by_text("Раздел будет назначен NVR", exact=False)).to_be_visible()

            harness.page.once("dialog", lambda dialog: asyncio.create_task(dialog.accept("CREATE /dev/sdb")))
            await harness.page.locator(".storage-disk-create-form button.button.primary").click()
            await expect(harness.page.get_by_text("Назначение раздела сохранено.", exact=False).first).to_be_visible()
            assert any(item.get("device") == "/dev/sdb1" and item.get("purpose") == "nvr" for item in posted)
            assert any(item.get("purpose") == "nvr" and item.get("operation") == "partition.create" and
                       not item.get("dry_run") for item in posted)
            assert not any(item.get("operation") == "format" for item in posted)
            await harness.close()
        finally:
            await browser.close()
    print("PASS: Existing and new partitions both accept NVR; no format requested.")


if __name__ == "__main__":
    asyncio.run(main())
