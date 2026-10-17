package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/DeadSoulf/home-ai-core/internal/filedata"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func quotaFixture(t *testing.T) (http.Handler, *state.Store, state.NASFolderRecord) {
	t.Helper()
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now()
	if _, err := store.CreateOwner(ctx, "usr_owner", "owner", "Owner", "hash", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(ctx, "usr_friend", "friend", "Friend", "hash", "role_member", now); err != nil {
		t.Fatal(err)
	}
	poolRoot := t.TempDir()
	pool, err := store.CreateNASPool(ctx, "Main", poolRoot, "/dev/test", "uuid-test", "usr_owner", now)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := store.CreateNASFolder(ctx, pool.ID, "Personal", "private", "usr_friend", "usr_owner", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(poolRoot, ".home-ai", folder.RelativePath), 0o750); err != nil {
		t.Fatal(err)
	}
	sec := defaultFakeSecurity()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := New("test", logger, store, sec, nil, nil, nil, nil, realtime.New("test", logger))
	old := suspendSMBAccess
	suspendSMBAccess = func(context.Context) (bool, error) { return false, nil }
	t.Cleanup(func() { suspendSMBAccess = old })
	folder, _ = store.NASFolder(ctx, folder.ID)
	return handler, store, folder
}
func quotaRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	if strings.Contains(path, "/chunk") {
		req.Header.Set("Upload-Offset", "0")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestDeleteEmptyLogicalFolderAndItsGrants(t *testing.T) {
	handler, store, folder := quotaFixture(t)
	root, err := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath)
	if err != nil {
		t.Fatal(err)
	}
	rec := quotaRequest(handler, http.MethodDelete, "/api/v1/files/folders/"+folder.ID, "{\"confirm_name\":\"Personal\"}")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete empty folder: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("empty physical folder remains: %v", err)
	}
	if _, err := store.NASFolder(context.Background(), folder.ID); !errors.Is(err, state.ErrNASFolderNotFound) {
		t.Fatalf("deleted folder metadata remains: %v", err)
	}
	grants, err := store.NASFolderAccess(context.Background(), folder.ID)
	if err != nil || len(grants) != 0 {
		t.Fatalf("stale permission grants after delete: %#v, %v", grants, err)
	}
}

func TestDeleteLogicalFolderRejectsNonEmptyAndWrongName(t *testing.T) {
	handler, store, folder := quotaFixture(t)
	root, err := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "important.txt")
	if err := os.WriteFile(target, []byte("preserve user data"), 0o640); err != nil {
		t.Fatal(err)
	}
	url := "/api/v1/files/folders/" + folder.ID
	for _, tc := range []struct{ body, code string }{
		{"{\"confirm_name\":\"incorrect\"}", "file_folder_confirmation_mismatch"},
		{"{\"confirm_name\":\"Personal\"}", "file_folder_not_empty"},
	} {
		rec := quotaRequest(handler, http.MethodDelete, url, tc.body)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), tc.code) {
			t.Fatalf("guard %q: %d %s", tc.code, rec.Code, rec.Body)
		}
	}
	if _, err := store.NASFolder(context.Background(), folder.ID); err != nil {
		t.Fatalf("rejected delete removed folder metadata: %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "preserve user data" {
		t.Fatalf("rejected delete altered user file: %q %v", data, err)
	}
}

func TestDeleteLogicalFolderRejectsHardQuota(t *testing.T) {
	handler, store, folder := quotaFixture(t)
	if err := store.UpdateNASFolderManagement(context.Background(), folder.ID, folder.Name,
		2<<20, 2<<20, nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rec := quotaRequest(handler, http.MethodDelete, "/api/v1/files/folders/"+folder.ID,
		"{\"confirm_name\":\"Personal\"}")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "file_folder_hard_quota_active") {
		t.Fatalf("active hard quota should block delete: %d %s", rec.Code, rec.Body)
	}
	if _, err := store.NASFolder(context.Background(), folder.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteVisibleEmptyFolderExplainsHiddenUploadAndTrash(t *testing.T) {
	for _, tc := range []struct {
		name string
		artifact string
		wantField string
	}{
		{"hidden upload artifact", ".home-ai-upload-abandoned.part", "upload_artifacts"},
		{"hidden trash payload", filepath.Join(".home-ai-trash", "items", "orphan"), "trash_items"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler,store,folder:=quotaFixture(t)
			root,err:=filedata.FolderRoot(folder.PoolRoot,folder.RelativePath)
			if err!=nil {t.Fatal(err)}
			target:=filepath.Join(root,tc.artifact)
			if err:=os.MkdirAll(filepath.Dir(target),0o750);err!=nil{t.Fatal(err)}
			if err:=os.WriteFile(target,[]byte("must not remove"),0o640);err!=nil{t.Fatal(err)}
			// The browser's ordinary file listing is empty because internal
			// trash and unfinished-upload files are intentionally hidden.
			listing:=quotaRequest(handler,http.MethodGet,"/api/v1/files/folders/"+folder.ID+"/entries","")
			if listing.Code!=http.StatusOK || !strings.Contains(listing.Body.String(),`"entries":[]`) {
				t.Fatalf("ordinary listing should look empty: %d %s",listing.Code,listing.Body)
			}
			rec:=quotaRequest(handler,http.MethodDelete,"/api/v1/files/folders/"+folder.ID,
				`{"confirm_name":"Personal"}`)
			if rec.Code!=http.StatusConflict {t.Fatalf("delete must be refused: %d %s",rec.Code,rec.Body)}
			var reply struct {
				Error struct {
					Code string `json:"code"`
					Details struct {
						Blockers filedata.FolderRemovalBlockers `json:"blockers"`
					} `json:"details"`
				} `json:"error"`
			}
			if err:=json.Unmarshal(rec.Body.Bytes(),&reply);err!=nil{t.Fatal(err)}
			if reply.Error.Code!="file_folder_not_empty"{t.Fatalf("unexpected error: %#v",reply)}
			switch tc.wantField {
			case "upload_artifacts":
				if reply.Error.Details.Blockers.UploadArtifacts!=1{t.Fatalf("expected hidden upload diagnostics: %#v",reply)}
			case "trash_items":
				if reply.Error.Details.Blockers.TrashItems!=1{t.Fatalf("expected hidden trash diagnostics: %#v",reply)}
			}
			if _,err:=os.Lstat(target);err!=nil{t.Fatalf("data lost after rejected delete: %v",err)}
			if _,err:=store.NASFolder(context.Background(),folder.ID);err!=nil{t.Fatalf("folder metadata lost: %v",err)}
		})
	}
}

func TestDeleteLogicalFolderRequiresFilesManage(t *testing.T) {
	_, store, folder := quotaFixture(t)
	sec := defaultFakeSecurity()
	limited := make([]string, 0, len(sec.actor.Permissions))
	for _, name := range sec.actor.Permissions {
		if name != "files.manage" {
			limited = append(limited, name)
		}
	}
	sec.actor.Permissions = limited
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := New("test", logger, store, sec, nil, nil, nil, nil, realtime.New("test", logger))
	rec := quotaRequest(handler, http.MethodDelete,
		"/api/v1/files/folders/"+folder.ID, "{\"confirm_name\":\"Personal\"}")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-manager delete: %d %s", rec.Code, rec.Body)
	}
	if _, err := store.NASFolder(context.Background(), folder.ID); err != nil {
		t.Fatalf("unauthorized delete altered metadata: %v", err)
	}
	if _, err := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath); err != nil {
		t.Fatalf("unauthorized delete altered folder path: %v", err)
	}
}

func TestFolderSettingsKeepPathsOwnerAndUnrelatedPermissions(t *testing.T) {
	handler, store, folder := quotaFixture(t)
	ctx := context.Background()
	other, err := store.CreateNASFolder(ctx, folder.PoolID, "Other", "shared", "", "usr_owner", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.GrantUserResourcePermission(ctx, "usr_friend", "files.read", "file_folder", other.ID); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"Renamed","quota_bytes":1000,"enforce_smb":false,"access":[]}`
	rec := quotaRequest(handler, "PUT", "/api/v1/files/folders/"+folder.ID+"/settings", body)
	if rec.Code != 200 {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	current, err := store.NASFolder(ctx, folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Name != "Renamed" || current.RelativePath != folder.RelativePath || current.OwnerUserID != folder.OwnerUserID || current.QuotaBytes != 1000 {
		t.Fatalf("identity/quota changed incorrectly: %+v", current)
	}
	access, err := store.NASFolderAccess(ctx, folder.ID)
	if err != nil || len(access) != 1 || !access[0].Read || !access[0].Write {
		t.Fatalf("private owner lost access: %+v %v", access, err)
	}
	unrelated, err := store.NASFolderAccess(ctx, other.ID)
	if err != nil || len(unrelated) != 1 || !unrelated[0].Read {
		t.Fatalf("unrelated grant removed: %+v %v", unrelated, err)
	}
}

func TestConcurrentReservationsCannotOverbookPersonalQuota(t *testing.T) {
	handler, store, folder := quotaFixture(t)
	if err := store.SetNASUserQuota(context.Background(), folder.OwnerUserID, 100, time.Now()); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	codes := make(chan int, 2)
	for _, name := range []string{"a", "b"} {
		group.Add(1)
		go func(name string) {
			defer group.Done()
			body := `{"path":"` + name + `","total_bytes":70}`
			codes <- quotaRequest(handler, "POST", "/api/v1/files/folders/"+folder.ID+"/uploads", body).Code
		}(name)
	}
	group.Wait()
	close(codes)
	success, rejected := 0, 0
	for code := range codes {
		if code == 201 {
			success++
		} else if code == 507 {
			rejected++
		} else {
			t.Fatalf("unexpected reservation status %d", code)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("overbooked: success=%d rejected=%d", success, rejected)
	}
	root, _ := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath)
	uploads, _ := filedata.ListUploads(root)
	chunk := quotaRequest(handler, "PUT", "/api/v1/files/folders/"+folder.ID+"/uploads/"+uploads[0].ID+"/chunk?offset=0", strings.Repeat("x", 70))
	if chunk.Code != 200 {
		t.Fatalf("own reservation could not be consumed: %d %s", chunk.Code, chunk.Body)
	}
	complete := quotaRequest(handler, "POST", "/api/v1/files/folders/"+folder.ID+"/uploads/"+uploads[0].ID+"/complete", "")
	if complete.Code != 200 {
		t.Fatalf("complete: %d %s", complete.Code, complete.Body)
	}
	quota := quotaRequest(handler, "PUT", "/api/v1/files/users/"+folder.OwnerUserID+"/quota", `{"quota_bytes":60}`)
	if quota.Code != 409 {
		t.Fatalf("quota below usage: %d %s", quota.Code, quota.Body)
	}
}

func TestUploadedActiveContentIsAlwaysAnAttachment(t *testing.T) {
	handler, _, folder := quotaFixture(t)
	payload := `<script>document.body.textContent=localStorage.getItem('home-ai-core.csrf')</script>`
	rec := quotaRequest(handler, "PUT", "/api/v1/files/folders/"+folder.ID+"/content?path=active.html", payload)
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body)
	}
	rec = quotaRequest(handler, "GET", "/api/v1/files/folders/"+folder.ID+"/content?path=active.html", "")
	if rec.Code != 200 || rec.Body.String() != payload {
		t.Fatal("download changed", rec.Code, rec.Body)
	}
	if rec.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment;") || rec.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("active-content headers missing: %+v", rec.Header())
	}
}

func TestSMBWritesRequireVerifiedPoolAndUserBudgets(t *testing.T) {
	oldInspect := inspectFolderHardQuota
	oldCapacity := readFilePoolCapacity
	defer func() { inspectFolderHardQuota = oldInspect; readFilePoolCapacity = oldCapacity }()
	inspectFolderHardQuota = func(context.Context, string, uint32) (storage.QuotaStatus, error) {
		return storage.QuotaStatus{LimitBytes: 64 << 20, UsedBytes: 4 << 20}, nil
	}
	readFilePoolCapacity = func(string) (filedata.Capacity, error) {
		return filedata.Capacity{TotalBytes: 1 << 30, FreeBytes: 1 << 29}, nil
	}
	folder := state.NASFolderRecord{ID: "folder", PoolID: "pool", PoolRoot: "/test", PoolReservePercent: 5, OwnerUserID: "user", Kind: "private", ProjectID: 1, HardQuotaBytes: 64 << 20}
	allowed := smbWritableFolders(context.Background(), []state.NASFolderRecord{folder}, map[string]int64{"user": 64 << 20})
	if !allowed[folder.ID] {
		t.Fatal("verified budget denied")
	}
	extra := folder
	extra.ID = "other"
	extra.ProjectID = 2
	allowed = smbWritableFolders(context.Background(), []state.NASFolderRecord{folder, extra}, map[string]int64{"user": 64 << 20})
	if allowed[folder.ID] || allowed[extra.ID] {
		t.Fatal("aggregate personal quota bypassed")
	}
	extra.HardQuotaBytes = 0
	allowed = smbWritableFolders(context.Background(), []state.NASFolderRecord{folder, extra}, nil)
	if !allowed[folder.ID] || !allowed[extra.ID] {
		t.Fatal("unlimited folder lost the existing broker pool quota protection")
	}
	extra.QuotaBytes = 64 << 20
	allowed = smbWritableFolders(context.Background(), []state.NASFolderRecord{folder, extra}, nil)
	if allowed[extra.ID] {
		t.Fatal("finite folder limit can be bypassed without a project quota")
	}
}

func TestQuotaEndpointDoesNotExposeCredentials(t *testing.T) {
	handler, _, _ := quotaFixture(t)
	rec := quotaRequest(handler, "GET", "/api/v1/files/owners", "")
	var response map[string]any
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &response) != nil {
		t.Fatal(rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "permissions") {
		t.Fatal("owner selector exposes security details")
	}
}
