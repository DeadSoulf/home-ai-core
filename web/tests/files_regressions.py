"""End-to-end browser checks for Files, against a synthetic in-memory API only.

All mounts, uploads, deletes, quotas and SMB calls are intercepted. There are
no real disks, network shares, users or production services behind this suite.
"""
import asyncio
import copy
import hashlib
import json
from urllib.parse import urlparse, parse_qs

from playwright.async_api import async_playwright, expect
from browser_regressions import Harness, OUT, RESULTS

FOLDER = {
    "id": "fld-test", "pool_id": "pool-test", "pool_name": "Main",
    "name": "Документы", "kind": "shared", "relative_path": "shared/fld-test",
    "can_read": True, "can_write": True, "quota_bytes": 0,
    "hard_quota_bytes": 0, "used_bytes": 1024, "reserved_bytes": 0,
    "usage_known": True,
}
POOL = {
    "id": "pool-test", "name": "Main", "root_path": "/mnt/home-ai-core/test",
    "capacity_known": True, "capacity_state": "ok", "size_bytes": 10000000000,
    "free_bytes": 5000000000, "reserve_bytes": 1000000000,
    "warning_bytes": 2000000000, "reserve_percent": 10, "warning_percent": 20,
}
OWNER = {"id": "usr-owner", "username": "owner", "display_name": "Home Owner"}
TIME = "2026-10-17T00:00:00Z"

def entry(path, kind="file", size=5):
    return {"name": path.rsplit("/", 1)[-1], "path": path, "kind": kind, "size_bytes": size, "modified_at": TIME}

def jsonresponse(body):
    return json.dumps(body, ensure_ascii=False)

class FilesHarness(Harness):
    def __init__(self, browser, **kwargs):
        super().__init__(browser)
        self.actor["permissions"] = [
            "security.self.read", "files.manage", "storage.manage", "system.read", "network.read",
            "security.users.read",
        ]
        self.folders = [copy.deepcopy(FOLDER)]
        self.pools = [copy.deepcopy(POOL)]
        self.owners = [copy.deepcopy(OWNER)]
        self.assignments = [{
            "device": "/dev/sdb1", "purpose": "files", "present": True,
            "filesystem": "ext4", "mountpoints": ["/mnt/home-ai-core/test"],
            "free_known": True, "free_bytes": 5000000000,
            "in_use": True, "used_by": [{"type": "file_pool", "id": "pool-test", "name": "Main"}],
        }, {
            "device": "/dev/sdc1", "purpose": "files", "present": True,
            "filesystem": "ext4", "mountpoints": [], "free_known": True,
            "free_bytes": 2000000000, "in_use": False, "used_by": [],
        }]
        self.smb = {
            "available": True, "active": True, "error": "",
            "hostname": "home-test-server", "workgroup": "WORKGROUP",
            "hard_quota_ready": True, "hard_quota_error": "",
            "users": [{"user_id": "usr-owner", "username": "owner",
                       "display_name": "Home Owner", "smb_username": "owner", "configured": False}],
            "shares": [{"folder_id": "fld-test", "folder_name": "Документы",
                        "share_name": "Documents", "unc": "\\\\home-test-server\\Documents", "writable": True}],
        }
        self.entries = {"fld-test": {"": [entry("Reports", "directory"), entry("notes.txt")],
                                      "Reports": [entry("Reports/report.txt")]}}
        self.trash = {"fld-test": []}
        self.uploads = {"fld-test": []}
        self.operations = []
        self.next_id = 1
        self.api_status = {}
        self.api_errors = {}
        for key, value in kwargs.items():
            setattr(self, key, value)

    async def route(self, route):
        req = route.request
        raw = urlparse(req.url)
        path, method = raw.path, req.method
        if not (path.startswith("/api/v1/files") or path.startswith("/api/v1/storage/purposes") or path == "/api/v1/storage/operation"):
            return await super().route(route)
        if method not in ("GET", "HEAD"):
            try: payload = req.post_data_json
            except Exception: payload = None
            self.requests.append((method, path, payload))
        else: payload = None
        self.operations.append((method, path))
        async def respond(body=None, status=200):
            await route.fulfill(status=status, content_type="application/json",
                body=jsonresponse(body if body is not None else {"error": {"code": "test_failure", "message": "isolated fixture error"}}))
        if path in self.api_errors:
            status, code, message, request_id = self.api_errors[path]
            return await respond({"error": {"code": code, "message": message, "request_id": request_id}}, status)
        if path in self.api_status:
            return await respond(status=self.api_status[path])

        if path == "/api/v1/files/folders":
            if method == "GET": return await respond({"folders": self.folders})
            if method == "POST":
                new = {**FOLDER, "id": f"fld-new-{self.next_id}", "name": payload["name"],
                       "kind": payload["kind"], "pool_id": payload["pool_id"],
                       "owner_user_id": payload.get("owner_user_id")}
                self.next_id += 1
                self.folders.append(new); self.entries[new["id"]] = {"": []}; self.trash[new["id"]] = []
                self.uploads[new["id"]] = []
                return await respond({"folder": new, "warning": ""}, 201)
        if path == "/api/v1/files/owners" and method == "GET":
            return await respond({"users": self.owners})
        if path == "/api/v1/files/pools":
            if method == "GET": return await respond({"pools": self.pools})
            if method == "POST":
                new = {**POOL, "id": f"pool-new-{self.next_id}", "name": payload["name"], "root_path": payload["root_path"]}
                self.next_id += 1; self.pools.append(new)
                return await respond({"pool": new}, 201)
        if path.startswith("/api/v1/files/pools/") and path.endswith("/capacity-policy") and method == "PATCH":
            pool = next(p for p in self.pools if p["id"] == path.split("/")[5])
            pool["reserve_percent"] = payload["reserve_percent"]
            pool["warning_percent"] = payload["warning_percent"]
            return await respond({"pool": pool})
        if path == "/api/v1/storage/purposes" and method == "GET":
            return await respond({"assignments": self.assignments})
        if path == "/api/v1/storage/operation" and method == "POST":
            for storage in self.assignments:
                if storage["device"] == payload["device"]:
                    storage["mountpoints"] = ["/mnt/home-ai-core/sdc1"]
            return await respond({"mountpoint": "/mnt/home-ai-core/sdc1", "message": "mounted"})
        if path == "/api/v1/files/smb":
            if method == "GET": return await respond({"smb": self.smb})
        if path == "/api/v1/files/smb/operation" and method == "POST":
            if payload["operation"] == "install":
                self.smb["available"] = True
                self.smb["hard_quota_ready"] = True
            elif payload["operation"] == "apply":
                self.smb["workgroup"] = payload["workgroup"]
            elif payload["operation"] == "set_password":
                self.smb["users"][0]["configured"] = True
            return await respond({"message": "updated"})
        if path.startswith("/api/v1/files/folders/"):
            parts = path.removeprefix("/api/v1/files/folders/").split("/")
            folder_id = parts[0]
            kind = "/".join(parts[1:])
            tree = self.entries.setdefault(folder_id, {"": []})
            if kind == "" and method == "DELETE":
                folder = next((f for f in self.folders if f["id"] == folder_id), None)
                if not folder:
                    return await respond({"error": {"code": "file_folder_not_found", "message": "folder missing"}}, 404)
                if payload.get("confirm_name") != folder["name"]:
                    return await respond({"error": {"code": "file_folder_confirmation_mismatch", "message": "confirmation mismatch"}}, 409)
                if folder.get("hard_quota_bytes", 0):
                    return await respond({"error": {"code": "file_folder_hard_quota_active", "message": "hard quota must be disabled"}}, 409)
                if any(tree.values()) or self.trash.get(folder_id) or self.uploads.get(folder_id):
                    return await respond({"error": {"code": "file_folder_not_empty", "message": "folder contains files, uploads or trash"}}, 409)
                self.folders = [f for f in self.folders if f["id"] != folder_id]
                del self.entries[folder_id]
                return await respond({"warning": ""})
            if kind == "settings":
                if method == "GET": return await respond({"folder": next(f for f in self.folders if f["id"]==folder_id), "access": []})
                if method == "PUT":
                    next(f for f in self.folders if f["id"]==folder_id).update({
                        "name": payload["name"], "quota_bytes": payload["quota_bytes"],
                        "hard_quota_bytes": payload["quota_bytes"] if payload["enforce_smb"] else 0,
                    })
                    return await respond({"warning": ""})
            if kind == "entries" and method == "GET":
                rel = parse_qs(raw.query).get("path", [""])[0]
                return await respond({"entries": tree.setdefault(rel, [])})
            if kind == "directories" and method == "POST":
                rel = payload["path"]; parent = rel.rpartition("/")[0]
                tree.setdefault(parent, []).append(entry(rel, "directory"))
                tree[rel] = []
                return await respond({"path": rel}, 201)
            if kind == "content" and method == "GET":
                return await route.fulfill(status=200, content_type="application/octet-stream",
                                           body=b"Isolated fixture file\\n")
            if kind == "entry" and method == "DELETE":
                rel = parse_qs(raw.query).get("path", [""])[0]
                parent = rel.rpartition("/")[0]
                found = next((e for e in tree.get(parent, []) if e["path"] == rel), None)
                if found:
                    tree[parent] = [e for e in tree[parent] if e["path"] != rel]
                    self.trash.setdefault(folder_id, []).append({"id": f"trash-{self.next_id}", "original_path": rel,
                        "name": found["name"], "kind": found["kind"], "deleted_at": TIME, "size_bytes": found["size_bytes"]})
                    self.next_id += 1
                return await route.fulfill(status=204)
            if kind == "move" and method == "POST":
                old, new = payload["from_path"], payload["to_path"]
                source = tree.setdefault(old.rpartition("/")[0], [])
                found = next(e for e in source if e["path"] == old)
                source.remove(found)
                tree.setdefault(new.rpartition("/")[0], []).append({**found, "path": new, "name": new.rsplit("/", 1)[-1]})
                return await respond({"path": new})
            if kind == "trash" and method == "GET":
                return await respond({"trash": self.trash.setdefault(folder_id, [])})
            if kind.startswith("trash/"):
                trash_id=parts[2]
                records = self.trash.setdefault(folder_id, [])
                found = next((e for e in records if e["id"] == trash_id), None)
                if found is None: return await respond(status=404)
                if kind.endswith("/restore") and method == "POST":
                    records.remove(found); old=found["original_path"]; tree.setdefault(old.rpartition("/")[0], []).append(entry(old,found["kind"],found.get("size_bytes",0)))
                    return await respond({"path":old})
                if method == "DELETE":
                    records.remove(found); return await route.fulfill(status=204)
            if kind == "uploads":
                if method == "GET": return await respond({"uploads": self.uploads.setdefault(folder_id, [])})
                if method == "POST":
                    session = {"id": f"upload-{self.next_id}", "path": payload["path"],
                        "total_bytes": payload["total_bytes"], "received_bytes": 0,
                        "client_fingerprint": payload.get("client_fingerprint"), "chunks": [],
                        "created_at": TIME, "updated_at": TIME}
                    self.next_id += 1; self.uploads.setdefault(folder_id, []).append(session)
                    return await respond({"upload": session}, 201)
            if kind.startswith("uploads/"):
                upload_id=parts[2]
                sessions=self.uploads.setdefault(folder_id, [])
                session=next((e for e in sessions if e["id"] == upload_id), None)
                if session is None:return await respond(status=404)
                if len(parts)==3 and method=="GET":return await respond({"upload":session})
                if len(parts)==3 and method=="DELETE":
                    sessions.remove(session);return await route.fulfill(status=204)
                if kind.endswith("/chunk") and method=="PUT":
                    offset=int(req.headers.get("upload-offset","-1"))
                    if offset!=session["received_bytes"]:return await respond(status=409)
                    session["received_bytes"]=session["total_bytes"]
                    session["chunks"].append({"offset":offset,"size":session["total_bytes"]-offset,
                        "sha256":req.headers.get("x-chunk-sha256","")})
                    return await respond({"upload":session})
                if kind.endswith("/complete") and method=="POST":
                    sessions.remove(session)
                    rel=session["path"]
                    tree.setdefault(rel.rpartition("/")[0], []).append(entry(rel,"file",session["total_bytes"]))
                    return await respond({"file":{"path":rel,"size_bytes":session["total_bytes"],
                        "sha256":hashlib.sha256(rel.encode()).hexdigest()}})
        self.unhandled.append((method,path))
        return await respond(status=404)

async def run_suite(browser):
    async def basic_folders():
        h=await FilesHarness(browser).start("/files")
        await expect(h.page.locator(".ux-folder-card")).to_have_count(1)
        await expect(h.page.locator(".ux-folder-card")).to_contain_text("Документы")
        await expect(h.page.locator(".ux-folder-card")).to_contain_text("1.00 KiB")
        await h.page.locator(".ux-folder-card button.primary").click()
        await expect(h.page.locator(".file-browser-toolbar")).to_be_visible()
        await expect(h.page.get_by_role("row").filter(has_text="notes.txt")).to_have_count(1)
        await h.page.get_by_role("button",name="Reports",exact=True).click()
        await expect(h.page.get_by_text("report.txt",exact=True)).to_be_visible()
        await h.page.locator(".file-browser-toolbar button").first.click()
        await expect(h.page.get_by_role("row").filter(has_text="notes.txt")).to_have_count(1)
        assert not h.unhandled, h.unhandled
        await h.close()

    async def directories_and_move():
        h=await FilesHarness(browser).start("/files")
        await h.page.locator(".ux-folder-card button.primary").click()
        form=h.page.locator(".file-browser-actions form")
        await form.locator("input").fill("NewDirectory")
        await form.locator("button").click()
        await expect(h.page.get_by_role("button",name="NewDirectory",exact=True)).to_be_visible()
        row=h.page.get_by_role("row").filter(has_text="notes.txt")
        h.page.once("dialog",lambda dialog: asyncio.create_task(dialog.accept("renamed.txt")))
        await row.locator("button").nth(1).click()
        await expect(h.page.get_by_role("row").filter(has_text="renamed.txt")).to_have_count(1)
        h.page.once("dialog",lambda dialog: asyncio.create_task(dialog.accept()))
        await h.page.get_by_role("row").filter(has_text="renamed.txt").locator("button").last.click()
        await expect(h.page.get_by_role("row").filter(has_text="renamed.txt")).to_have_count(0)
        await h.page.locator(".file-browser-toolbar button").nth(2).click()
        await expect(h.page.get_by_role("row").filter(has_text="renamed.txt")).to_have_count(1)
        assert len([x for x in h.requests if x[1].endswith("/directories")])==1
        await h.close()

    async def restore_and_purge():
        h=await FilesHarness(browser).start("/files")
        await h.page.locator(".ux-folder-card button.primary").click()
        h.page.once("dialog",lambda dialog: asyncio.create_task(dialog.accept()))
        await h.page.get_by_role("row").filter(has_text="notes.txt").locator("button").last.click()
        await h.page.locator(".file-browser-toolbar button").nth(2).click()
        trash_row=h.page.get_by_role("row").filter(has_text="notes.txt")
        await expect(trash_row).to_be_visible()
        await trash_row.locator("button").first.click()
        await expect(h.page.get_by_role("row").filter(has_text="notes.txt")).to_have_count(1)
        h.page.once("dialog",lambda dialog: asyncio.create_task(dialog.accept()))
        await h.page.get_by_role("row").filter(has_text="notes.txt").locator("button").last.click()
        trash_row=h.page.get_by_role("row").filter(has_text="notes.txt")
        h.page.once("dialog",lambda dialog: asyncio.create_task(dialog.accept()))
        await trash_row.locator("button").last.click()
        assert not h.trash["fld-test"]
        await h.close()

    async def downloads_and_uploads():
        h=await FilesHarness(browser).start("/files")
        await h.page.locator(".ux-folder-card button.primary").click()
        row=h.page.get_by_role("row").filter(has_text="notes.txt")
        async with h.page.expect_download() as download:
            await row.locator("button").first.click()
        d=await download.value
        assert d.suggested_filename=="notes.txt",d.suggested_filename
        await h.page.locator(".file-upload-button input").set_input_files({
            "name":"uploaded.txt","mimeType":"text/plain","buffer":b"hello from isolated test"
        })
        await expect(h.page.get_by_text("uploaded.txt",exact=True)).to_be_visible()
        assert len([x for x in h.requests if x[1].endswith("/chunk")])==1
        assert len([x for x in h.requests if x[1].endswith("/complete")])==1
        assert not h.unhandled,h.unhandled
        await h.close()

    async def folder_settings():
        h=await FilesHarness(browser).start("/files")
        await h.page.locator(".ux-folder-card button.secondary").click()
        panel=h.page.locator("section.panel").filter(has=h.page.locator("input[type=number]"))
        await expect(panel).to_be_visible()
        await panel.locator('input[type=number]').fill("2")
        await panel.locator('input[type=text]').first.fill("Отчёты") if await panel.locator('input[type=text]').count() else await panel.locator('input').first.fill("Отчёты")
        await panel.locator('button').last.click()
        await expect(h.page.locator(".ux-folder-card")).to_contain_text("Отчёты")
        assert any(x[0]=="PUT" and x[1].endswith("/settings") for x in h.requests)
        await h.close()

    async def admin_storage_and_smb():
        h=await FilesHarness(browser).start("/files/storage")
        await expect(h.page.get_by_text("/mnt/home-ai-core/test",exact=False).first).to_be_visible()
        mount=h.page.get_by_role("row").filter(has_text="/dev/sdc1")
        await mount.get_by_role("button").click()
        assert len([x for x in h.requests if x[1]=="/api/v1/storage/operation"])==1
        policy=h.page.get_by_role("row").filter(has_text="Main")
        await policy.locator('input[name=reserve_percent]').fill("15")
        await policy.locator('input[name=warning_percent]').fill("25")
        await policy.locator("button[type=submit]").click()
        assert next(p for p in h.pools if p["id"]=="pool-test")["reserve_percent"]==15
        await h.close()
        h=await FilesHarness(browser).start("/files/windows")
        await expect(h.page.locator(".smb-password-form")).to_be_visible()
        await h.page.locator(".smb-password-form input[type=password]").fill("long-password-example")
        await h.page.locator(".smb-password-form button[type=submit]").click()
        assert h.smb["users"][0]["configured"]
        assert any(x[1]=="/api/v1/files/smb/operation" for x in h.requests)
        await h.close()

    async def read_only_role():
        h=FilesHarness(browser)
        h.actor["permissions"]=["security.self.read"]
        h.folders[0]["can_write"]=False
        await h.start("/files")
        await expect(h.page.locator(".ux-folder-card button.primary")).to_be_visible()
        await expect(h.page.locator(".ux-folder-card button.secondary")).to_have_count(0)
        await h.page.locator(".ux-folder-card button.primary").click()
        await expect(h.page.locator(".file-browser-toolbar")).to_be_visible()
        assert await h.page.locator(".file-browser-actions").count()==0
        assert await h.page.locator(".file-section-tabs").count()==0
        await h.close()

    async def ancillary_api_failure():
        # This should NOT hide accessible file folders. The current React
        # Promise.all() fails the whole page when user/pool metadata is unavailable.
        h=FilesHarness(browser)
        h.api_status["/api/v1/files/owners"]=503
        await h.start("/files")
        await expect(h.page.locator(".ux-folder-card")).to_have_count(1,timeout=4500)
        await h.close()

    async def create_folder_and_pool():
        h=await FilesHarness(browser).start("/files")
        form=h.page.locator(".user-form").last
        await form.locator("select").first.select_option("pool-test")
        await form.locator("input").first.fill("Shared-Test")
        await form.locator("select").nth(1).select_option("shared")
        await form.locator("button").last.click()
        await expect(h.page.locator(".ux-folder-card")).to_have_count(2)
        assert any(x[1]=="/api/v1/files/folders" and x[0]=="POST" for x in h.requests)
        await h.close()
        h=await FilesHarness(browser).start("/files/storage")
        await h.page.get_by_role("row").filter(has_text="/dev/sdc1").get_by_role("button").click()
        form=h.page.locator(".user-form").first
        await form.locator("input").first.fill("New-Test-Pool")
        await form.locator("select").first.select_option("/mnt/home-ai-core/sdc1")
        await form.locator("button").last.click()
        assert any(p["name"]=="New-Test-Pool" for p in h.pools),h.pools
        await h.close()

    async def install_and_apply_smb():
        h=FilesHarness(browser)
        h.smb["available"]=False; h.smb["active"]=False
        await h.start("/files/windows")
        await h.page.locator(".smb-install button").click()
        await expect(h.page.locator(".smb-actions input")).to_be_visible()
        await h.page.locator(".smb-actions input").fill("HOMEGROUP")
        await h.page.locator(".smb-actions button").click()
        assert h.smb["workgroup"]=="HOMEGROUP",h.smb
        assert [x[2]["operation"] for x in h.requests if x[1]=="/api/v1/files/smb/operation"]==["install","apply"]
        await h.close()

    async def missing_system_read():
        # Files managers can open /files/storage without system.read,
        # but GET /api/v1/storage/purposes rejects them. The page
        # must explain the access problem rather than say no disks exist.
        h=FilesHarness(browser)
        h.actor["permissions"].remove("system.read")
        h.api_status["/api/v1/storage/purposes"]=403
        await h.start("/files/storage")
        await expect(h.page.get_by_role("alert")).to_be_visible(timeout=4500)
        await h.close()

    async def missing_storage_manage():
        # Files managers do not automatically have storage.manage.
        # The mount button must not invite a guaranteed 403 response.
        h=FilesHarness(browser)
        h.actor["permissions"].remove("storage.manage")
        h.api_status["/api/v1/storage/operation"]=403
        await h.start("/files/storage")
        mount=h.page.get_by_role("row").filter(has_text="/dev/sdc1")
        await expect(mount.get_by_role("button")).to_be_disabled(timeout=4500)
        assert not any(x[1]=="/api/v1/storage/operation" for x in h.requests)
        await h.close()

    async def backend_storage_error():
        h=FilesHarness(browser)
        path="/api/v1/files/folders/fld-test/entries"
        h.api_errors[path]=(502,"file_folder_storage_unavailable",
                            "inspect logical folder root: permission denied","req-storage-502")
        await h.start("/files")
        await h.page.locator(".ux-folder-card button.primary").click()
        alert=h.page.locator(".error-notice")
        await expect(alert).to_contain_text("Хранилище папки недоступно")
        await expect(alert).not_to_contain_text("Сервер недоступен")
        await alert.locator("summary").click()
        await expect(alert).to_contain_text("HTTP 502")
        await expect(alert).to_contain_text("file_folder_storage_unavailable")
        await expect(alert).to_contain_text("req-storage-502")
        await expect(alert).to_contain_text("permission denied")
        assert not h.unhandled,h.unhandled
        await h.close()

    async def failed_folder_settings():
        h=FilesHarness(browser)
        path="/api/v1/files/folders/fld-test/settings"
        h.api_errors[path]=(503,"file_management_unavailable",
                            "folder management unavailable","req-settings-503")
        await h.start("/files")
        await h.page.locator(".ux-folder-card button.secondary").click()
        panel=h.page.locator("section.panel").filter(has_text="Настройки папки")
        await expect(panel.locator(".error-notice")).to_contain_text("Недоступны данные папок")
        await panel.locator(".error-notice summary").click()
        await expect(panel.locator(".error-notice")).to_contain_text("HTTP 503")
        await expect(panel.locator(".error-notice")).to_contain_text("req-settings-503")
        assert not h.unhandled,h.unhandled
        await h.close()

    async def unknown_file_http_error():
        h=FilesHarness(browser)
        path="/api/v1/files/folders/fld-test/directories"
        h.api_errors[path]=(502,"unmapped_files_failure","synthetic unknown error","req-unknown-502")
        await h.start("/files")
        await h.page.locator(".ux-folder-card button.primary").click()
        form=h.page.locator(".file-browser-actions form")
        await form.locator("input").fill("NewDirectory")
        await form.locator("button").click()
        alert=h.page.locator(".error-notice")
        await expect(alert).to_contain_text("Core ответил ошибкой HTTP 502")
        await alert.locator("summary").click()
        await expect(alert).to_contain_text("unmapped_files_failure")
        await expect(alert).to_contain_text("req-unknown-502")
        assert not h.unhandled,h.unhandled
        await h.close()

    async def delete_empty_logical_folder():
        h=FilesHarness(browser)
        h.entries["fld-test"]={"": []}
        await h.start("/files")
        button=h.page.locator(".ux-folder-card button.danger")
        await expect(button).to_be_visible()
        h.page.once("dialog",lambda dialog: asyncio.create_task(dialog.accept("wrong-name")))
        await button.click()
        await expect(h.page.locator(".ux-folder-card")).to_have_count(1)
        assert not [r for r in h.requests if r[0]=="DELETE" and r[1].endswith("/fld-test")]
        h.page.once("dialog",lambda dialog: asyncio.create_task(dialog.accept("Документы")))
        await button.click()
        await expect(h.page.locator(".ux-folder-card")).to_have_count(0)
        requests=[r for r in h.requests if r[0]=="DELETE" and r[1].endswith("/fld-test")]
        assert len(requests)==1 and requests[0][2]=={"confirm_name":"Документы"},requests
        assert not h.unhandled,h.unhandled
        await h.close()

    async def delete_nonempty_is_blocked():
        h=await FilesHarness(browser).start("/files")
        h.page.once("dialog",lambda dialog: asyncio.create_task(dialog.accept("Документы")))
        await h.page.locator(".ux-folder-card button.danger").click()
        await expect(h.page.locator(".ux-folder-card")).to_have_count(1)
        await expect(h.page.locator(".error-notice")).to_contain_text("Папка не пуста")
        assert "fld-test" in h.entries
        assert not h.unhandled,h.unhandled
        await h.close()

    async def delete_folder_requires_admin():
        h=FilesHarness(browser)
        h.actor["permissions"]=["security.self.read","files.read","files.write"]
        await h.start("/files")
        await expect(h.page.locator(".ux-folder-card button.danger")).to_have_count(0)
        assert not [r for r in h.requests if r[0]=="DELETE"]
        await h.close()

    async def delete_with_pending_upload_or_trash_blocked():
        for which in ("upload","trash"):
            h=FilesHarness(browser)
            h.entries["fld-test"]={"": []}
            if which=="upload":
                h.uploads["fld-test"]=[{"id":"upload-pending"}]
            else:
                h.trash["fld-test"]=[{"id":"trash-pending"}]
            await h.start("/files")
            h.page.once("dialog",lambda dialog: asyncio.create_task(dialog.accept("Документы")))
            await h.page.locator(".ux-folder-card button.danger").click()
            await expect(h.page.locator(".ux-folder-card")).to_have_count(1)
            await expect(h.page.locator(".error-notice")).to_contain_text("Папка не пуста")
            assert h.folders,which
            await h.close()

    scenarios=[
        ("FILES-01: list/open/nested navigation",basic_folders),
        ("FILES-02: mkdir/move/delete/trash listing",directories_and_move),
        ("FILES-03: restore and permanent purge",restore_and_purge),
        ("FILES-04: download and chunked upload",downloads_and_uploads),
        ("FILES-05: folder settings and quota",folder_settings),
        ("FILES-06: storage mount/policy and SMB password",admin_storage_and_smb),
        ("FILES-07: read-only permission enforcement",read_only_role),
        ("FILES-08: partial API outage does not hide folders",ancillary_api_failure),
        ("FILES-09: create shared folder and new pool",create_folder_and_pool),
        ("FILES-10: SMB install and workgroup apply",install_and_apply_smb),
        ("FILES-11: file manager without system.read sees truthful storage error",missing_system_read),
        ("FILES-12: file manager without storage.manage cannot attempt mount",missing_storage_manage),
        ("FILES-13: failed storage API shows actionable cause and request ID",backend_storage_error),
        ("FILES-14: folder settings failure exposes API details",failed_folder_settings),
        ("FILES-15: unknown HTTP 502 is not falsely called offline",unknown_file_http_error),
        ("FILES-16: delete only a confirmed empty logical folder",delete_empty_logical_folder),
        ("FILES-17: delete refuses a folder with user files",delete_nonempty_is_blocked),
        ("FILES-18: non-managers cannot delete logical folders",delete_folder_requires_admin),
        ("FILES-19: unfinished uploads and trash prevent deletion",delete_with_pending_upload_or_trash_blocked),
    ]
    failures=[]
    for name, fn in scenarios:
        try:
            await fn()
            RESULTS.append({"name":name,"status":"passed"})
            print("PASS",name,flush=True)
        except Exception as ex:
            RESULTS.append({"name":name,"status":"failed","error":str(ex)})
            failures.append(name)
            print("FAIL",name,str(ex),flush=True)
    (OUT/"files-results.json").write_text(json.dumps({
        "environment":"production bundle, intercepted in-memory Files API only",
        "results":RESULTS[-len(scenarios):],
    },ensure_ascii=False,indent=2))
    if failures:
        raise AssertionError("Files failures: "+", ".join(failures))

async def main():
    OUT.mkdir(parents=True,exist_ok=True)
    async with async_playwright() as p:
        browser=await p.chromium.launch(headless=True,args=["--no-sandbox"])
        try: await run_suite(browser)
        finally: await browser.close()

if __name__=="__main__": asyncio.run(main())
