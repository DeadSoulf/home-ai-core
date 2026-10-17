package modules

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestUpgradeManifestSwitchesPinnedImageOnlyAfterHelperSuccess(t *testing.T) {
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil { t.Fatal(err) }
	defer db.Close()
	old := validManifest("nvr.module", "1.1.2")
	persistManifest(t, db, old, "enabled")
	newer := validManifest("nvr.module", "1.1.3")
	newer.Runtime.Docker.Image = "ghcr.io/deadsoulf/home-ai-core_dvr@sha256:" + strings.Repeat("b", 64)
	registry := NewRegistry(db, "0.1.186-dev")
	var gotPrevious, gotNext string
	registry.upgradeHelper = func(_ context.Context, id, previousImage, nextImage string) (helperModuleResult, error) {
		if id != "nvr.module" { t.Errorf("wrong module %q", id) }
		gotPrevious, gotNext = previousImage, nextImage
		return helperModuleResult{State:"running"},nil
	}
	item, err := registry.UpgradeManifest(ctx,newer)
	if err != nil || item.Manifest.Version!="1.1.3" || gotPrevious!=old.Runtime.Docker.Image || gotNext!=newer.Runtime.Docker.Image {
		t.Fatalf("upgrade=%#v err=%v old=%q new=%q",item,err,gotPrevious,gotNext)
	}
}
func TestUpgradeManifestRejectsBranchChangeAndPreservesOldRecord(t *testing.T) {
	ctx := context.Background()
	db, err := state.Open(ctx,t.TempDir());if err != nil { t.Fatal(err) };defer db.Close()
	old:=validManifest("demo", "1.1.2")
	persistManifest(t, db, old, "enabled")
	registry:=NewRegistry(db)
	called:=false
	registry.upgradeHelper=func(context.Context,string,string,string)(helperModuleResult,error){called=true;return helperModuleResult{},nil}
	next:=validManifest("demo", "2.0.0")
	next.Runtime.Docker.Image="example.com/demo@sha256:"+strings.Repeat("c",64)
	if _,err:=registry.UpgradeManifest(ctx,next); !errors.Is(err,ErrModuleUpgradeVersion) || called {
		t.Fatalf("unsafe major upgrade %v helper called=%v",err,called)
	}
	current,_:=registry.Get(ctx,"demo")
	if current.Manifest.Version!=old.Version {t.Fatal("installed manifest was overwritten")}
}
func TestUpgradeFailureDoesNotCommitNewManifest(t *testing.T) {
	ctx:=context.Background()
	db,err:=state.Open(ctx,t.TempDir());if err!=nil{t.Fatal(err)};defer db.Close()
	old:=validManifest("nvr.module","1.1.2")
	persistManifest(t,db,old,"enabled")
	next:=validManifest("nvr.module","1.1.3")
	next.Runtime.Docker.Image="example.com/demo@sha256:"+strings.Repeat("b",64)
	reg:=NewRegistry(db)
	reg.upgradeHelper=func(context.Context,string,string,string)(helperModuleResult,error) {
		return helperModuleResult{},errors.New("rollback complete")
	}
	if _,err:=reg.UpgradeManifest(ctx,next);err==nil{t.Fatal("failed helper treated as success")}
	current,_:=reg.Get(ctx,"nvr.module")
	if current.Manifest.Version!=old.Version || current.Status!="enabled" {t.Fatalf("old module lost: %+v",current)}
}
