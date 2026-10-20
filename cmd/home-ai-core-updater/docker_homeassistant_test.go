package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/homeassistant"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const haFakeDocker = `#!/usr/bin/env python3
import hashlib,json,os,sys
from pathlib import Path
args=sys.argv[1:]
state_path=Path(os.environ['HA_FAKE_STATE'])
state=json.loads(state_path.read_text()) if state_path.exists() else {'containers':{}}
with open(os.environ['HA_FAKE_LOG'],'a') as log: log.write(json.dumps(args)+'\n')
fails=set(os.environ.get('HA_FAKE_FAIL','').split(','))
def save(): state_path.write_text(json.dumps(state))
def die(message='No such container',code=1): print(message,file=sys.stderr); sys.exit(code)
def get(target):
    for c in state['containers'].values():
        if c['Id']==target or c['Name'].lstrip('/')==target: return c
    die()
def option(flag,default=None):
    return args[args.index(flag)+1] if flag in args else default
if not args: die('empty command')
cmd=args[0]
if cmd=='version':
    print(json.dumps({'Version':'28.0.0','Os':'linux','Arch':os.environ['HA_FAKE_ARCH']})); sys.exit()
if cmd=='info': print('Docker Desktop' if 'desktop' in fails else 'Debian GNU/Linux'); sys.exit()
if cmd=='pull':
    if 'pull' in fails: die('simulated pull failure')
    print('pulled immutable image'); sys.exit()
if cmd=='image' and args[1]=='inspect':
    print(json.dumps({'Os':'linux','Architecture':os.environ['HA_FAKE_ARCH'],'RepoDigests':[] if 'digest' in fails else [args[-1]]})); sys.exit()
if cmd=='create':
    name=option('--name'); candidate='upgrade-candidate' in name
    if candidate and 'create' in fails: die('simulated candidate create failure')
    labels={}; mounts=[]
    for i,a in enumerate(args[:-1]):
        if a=='--label': k,v=args[i+1].split('=',1); labels[k]=v
        if a=='--mount':
            options={}
            for part in args[i+1].split(','):
                if '=' in part: k,v=part.split('=',1); options[k]=v
                else: options[part]=True
            mounts.append({'Type':options['type'],'Source':options['src'],'Destination':options['dst'],'RW':not options.get('readonly',False)})
    cid=hashlib.sha256(name.encode()).hexdigest()
    c={'Id':cid,'Name':'/'+name,'candidate':candidate,'Config':{'Image':args[-1],'Labels':labels,'Healthcheck':{'Test':['CMD-SHELL',option('--health-cmd','')]}},'HostConfig':{'NetworkMode':option('--network'),'Privileged':'--privileged' in args},'State':{'Running':False,'Status':'created','Health':{'Status':'starting'}},'Mounts':mounts}
    state['containers'][cid]=c; save(); print(cid); sys.exit()
if cmd=='inspect':
    c=get(args[-1]); fmt=option('--format','')
    if fmt=='{{json .}}': print(json.dumps(c))
    elif fmt=='{{.Name}}': print(c['Name'])
    elif 'home-ai.managed' in fmt: print(c['Config']['Labels'].get('home-ai.managed','')+'|'+c['Config']['Labels'].get('home-ai.module.id',''))
    elif 'State.Health' in fmt: print(c['State']['Status']+'|'+c['State']['Health']['Status'])
    elif 'State.Status' in fmt: print(c['Id']+'|'+c['State']['Status'])
    elif 'Config.Image' in fmt: print(c['Config']['Image'])
    else: print(c['Id'])
    sys.exit()
if cmd in ('start','stop','restart','rm','rename'):
    c=get(args[-2] if cmd=='rename' else args[-1])
    if cmd in ('start','restart'):
        c['State']={'Running':True,'Status':'running','Health':{'Status':'unhealthy' if ('initial_health' in fails or (c['candidate'] and 'health' in fails)) else 'healthy'}}
        if c['candidate']:
            config=next(m['Source'] for m in c['Mounts'] if m['Destination']=='/config')
            Path(config,'database.marker').write_text('MIGRATED-BY-CANDIDATE')
        save()
        if c['candidate'] and 'start' in fails: die('candidate start returned error after starting')
    elif cmd=='stop':
        if c['candidate'] and 'stop_candidate' in fails: die('candidate cannot be stopped')
        c['State']['Running']=False; c['State']['Status']='exited'; save()
    elif cmd=='rm':
        if c['Name'].endswith('upgrade-previous') and 'commit_remove' in fails: die('previous cleanup failed')
        if c['State']['Running'] and '-f' not in args: die('refusing remove of running container')
        del state['containers'][c['Id']]; save()
    elif cmd=='rename':
        if not c['candidate'] and 'rename_old' in fails: die('old rename failed')
        if c['candidate'] and 'rename_candidate' in fails: die('candidate rename failed')
        target=args[-1]
        if any(x['Name']=='/'+target and x['Id']!=c['Id'] for x in state['containers'].values()): die('name already exists')
        c['Name']='/'+target; save()
        if c['candidate'] and 'rename_candidate_after' in fails: die('rename response lost after success')
    print(c['Id']); sys.exit()
die('unhandled fake docker command: '+repr(args),91)
`

func haTestEnvironment(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("native Linux helper")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("Python 3 is required for stateful fake Docker")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "docker")
	if err := os.WriteFile(binary, []byte(haFakeDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	oldRoot, oldPort, oldHealth := moduleDataRoot, haPortAvailable, haHealthWindow
	moduleDataRoot = filepath.Join(dir, "modules")
	if err := os.Mkdir(moduleDataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	haPortAvailable = func() error { return nil }
	haHealthWindow = 100 * time.Millisecond
	t.Cleanup(func() { moduleDataRoot = oldRoot; haPortAvailable = oldPort; haHealthWindow = oldHealth })
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HA_FAKE_ARCH", runtime.GOARCH)
	t.Setenv("HA_FAKE_STATE", filepath.Join(dir, "docker.json"))
	t.Setenv("HA_FAKE_LOG", filepath.Join(dir, "commands.jsonl"))
	t.Setenv("HA_FAKE_FAIL", "")
	t.Setenv("HOME_AI_MODULE_CATALOG_TOKEN", "")
	return binary, filepath.Join(dir, "commands.jsonl")
}

func haImage(letter string) string {
	return homeassistant.Repository + "@sha256:" + strings.Repeat(letter, 64)
}
func haInstallTest(t *testing.T) haState {
	t.Helper()
	status, _, err := installDockerModule(context.Background(), homeassistant.ModuleID, haImage("a"), os.Getuid(), os.Getgid())
	if err != nil || status.State != "running" {
		t.Fatalf("install=%#v err=%v", status, err)
	}
	state, err := haReadState()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.Directory, "database.marker"), []byte("ORIGINAL-DATABASE"), 0o600); err != nil {
		t.Fatal(err)
	}
	return state
}
func haRequest(op string) updaterhelper.Request {
	return updaterhelper.Request{Operation: op, ModuleID: homeassistant.ModuleID, Image: haImage("b"), PreviousImage: haImage("a")}
}

func TestHomeAssistantRejectsUnofficialImagesBeforeSideEffects(t *testing.T) {
	_, log := haTestEnvironment(t)
	for _, op := range []string{"docker.module.install", "docker.module.upgrade", "docker.module.upgrade.rollback", "docker.module.upgrade.commit"} {
		req := haRequest(op)
		req.Image = "ghcr.io/attacker/ha@sha256:" + strings.Repeat("a", 64)
		if _, _, err := performDockerModuleOperation(context.Background(), req, 1000, 1000); err == nil {
			t.Fatal("untrusted privileged image accepted")
		}
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("invalid request invoked Docker")
	}
}

func TestHomeAssistantInstallControlRemovePreservesConfig(t *testing.T) {
	binary, log := haTestEnvironment(t)
	state := haInstallTest(t)
	info, err := haInspect(context.Background(), binary, moduleContainerName(homeassistant.ModuleID))
	if err != nil || info.configDir != state.Directory {
		t.Fatalf("inspect=%#v err=%v", info, err)
	}
	for _, action := range []string{"stop", "start", "restart", "remove"} {
		if _, _, err := controlHomeAssistant(context.Background(), action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(state.Directory, "database.marker")); err != nil || string(data) != "ORIGINAL-DATABASE" {
		t.Fatalf("configuration lost: %s %v", data, err)
	}
	data, _ := os.ReadFile(log)
	text := string(data)
	if !strings.Contains(text, `"--privileged"`) || !strings.Contains(text, `"--health-cmd"`) || strings.Contains(text, "docker.sock") || strings.Contains(text, `"--user"`) {
		t.Fatalf("unexpected runtime flags: %s", text)
	}
	if _, _, err := installHomeAssistant(context.Background(), haImage("b"), firstModuleProgress(nil)); err == nil {
		t.Fatal("reinstall silently changed retained database image")
	}
	if _, _, err := installHomeAssistant(context.Background(), haImage("a"), firstModuleProgress(nil)); err != nil {
		t.Fatalf("exact-image reinstall failed: %v", err)
	}
}

func TestHomeAssistantDoesNotChangeGenericSandbox(t *testing.T) {
	_, log := haTestEnvironment(t)
	if _, _, err := installDockerModule(context.Background(), "demo", haImage("a"), os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(log)
	if strings.Contains(string(data), `"--privileged"`) || !strings.Contains(string(data), `"--read-only"`) {
		t.Fatalf("generic sandbox changed: %s", data)
	}
}

func TestHomeAssistantUpgradeCommitKeepsOriginalDatabase(t *testing.T) {
	binary, _ := haTestEnvironment(t)
	old := haInstallTest(t)
	status, _, err := upgradeHomeAssistant(context.Background(), haRequest("docker.module.upgrade"), firstModuleProgress(nil))
	if err != nil || status.State != "running" {
		t.Fatalf("upgrade=%#v err=%v", status, err)
	}
	current, err := haInspect(context.Background(), binary, moduleContainerName(homeassistant.ModuleID))
	if err != nil {
		t.Fatal(err)
	}
	if current.configDir == old.Directory {
		t.Fatal("candidate shares old database")
	}
	data, _ := os.ReadFile(filepath.Join(old.Directory, "database.marker"))
	if string(data) != "ORIGINAL-DATABASE" {
		t.Fatalf("original database migrated: %s", data)
	}
	data, _ = os.ReadFile(filepath.Join(current.configDir, "database.marker"))
	if string(data) != "MIGRATED-BY-CANDIDATE" {
		t.Fatal("candidate did not use copy")
	}
	if _, _, err := upgradeHomeAssistant(context.Background(), haRequest("docker.module.upgrade.commit"), firstModuleProgress(nil)); err != nil {
		t.Fatal(err)
	}
	active, err := haReadState()
	if err != nil || active.Image != haImage("b") || active.Directory != current.configDir {
		t.Fatalf("commit state=%#v err=%v", active, err)
	}
	if _, err := os.Stat(old.Directory); err != nil {
		t.Fatal("successful update deleted backup")
	}
}

func TestHomeAssistantUpgradeFailuresRestoreOriginalImageAndDatabase(t *testing.T) {
	for _, failure := range []string{"pull", "digest", "create", "start", "health", "rename_old", "rename_candidate", "rename_candidate_after"} {
		t.Run(failure, func(t *testing.T) {
			binary, _ := haTestEnvironment(t)
			old := haInstallTest(t)
			t.Setenv("HA_FAKE_FAIL", failure)
			if _, _, err := upgradeHomeAssistant(context.Background(), haRequest("docker.module.upgrade"), firstModuleProgress(nil)); err == nil {
				t.Fatal("failed candidate reported success")
			}
			current, err := haInspect(context.Background(), binary, moduleContainerName(homeassistant.ModuleID))
			if err != nil {
				t.Fatal(err)
			}
			if !current.State.Running || current.Config.Image != old.Image || current.configDir != old.Directory {
				t.Fatalf("old instance not restored: %#v", current)
			}
			data, _ := os.ReadFile(filepath.Join(old.Directory, "database.marker"))
			if string(data) != "ORIGINAL-DATABASE" {
				t.Fatalf("original changed: %s", data)
			}
		})
	}
}

func TestHomeAssistantRegistryFailureRollbackRestoresConfig(t *testing.T) {
	binary, _ := haTestEnvironment(t)
	old := haInstallTest(t)
	if _, _, err := upgradeHomeAssistant(context.Background(), haRequest("docker.module.upgrade"), firstModuleProgress(nil)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := upgradeHomeAssistant(context.Background(), haRequest("docker.module.upgrade.rollback"), firstModuleProgress(nil)); err != nil {
		t.Fatal(err)
	}
	current, err := haInspect(context.Background(), binary, moduleContainerName(homeassistant.ModuleID))
	if err != nil || current.configDir != old.Directory || current.Config.Image != old.Image {
		t.Fatalf("rollback=%#v err=%v", current, err)
	}
}

func TestHomeAssistantFailedCandidateStopDoesNotStartSecondInstance(t *testing.T) {
	binary, _ := haTestEnvironment(t)
	haInstallTest(t)
	t.Setenv("HA_FAKE_FAIL", "health,stop_candidate")
	if _, _, err := upgradeHomeAssistant(context.Background(), haRequest("docker.module.upgrade"), firstModuleProgress(nil)); err == nil || !strings.Contains(err.Error(), "rollback needs attention") {
		t.Fatalf("unsafe rollback result: %v", err)
	}
	old, err := haInspect(context.Background(), binary, moduleContainerName(homeassistant.ModuleID))
	if err != nil {
		t.Fatal(err)
	}
	if old.State.Running {
		t.Fatal("old instance restarted while candidate could not be stopped")
	}
}

func TestHomeAssistantBackupRefusesLinksAndProtectsPaths(t *testing.T) {
	haTestEnvironment(t)
	state := haInstallTest(t)
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(state.Directory, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := haCopyConfig(context.Background(), state.Directory); err == nil {
		t.Fatal("backup followed symlink")
	}
	if err := haValidateConfig(filepath.Dir(haRoot())); err == nil {
		t.Fatal("accepted external configuration directory")
	}
	bad := filepath.Join(haRoot(), "config-123")
	if err := os.Symlink(filepath.Dir(outside), bad); err != nil {
		t.Fatal(err)
	}
	if err := haValidateConfig(bad); err == nil {
		t.Fatal("accepted symlink configuration root")
	}
}

func TestHomeAssistantPortParser(t *testing.T) {
	header := "sl local_address rem_address st\n"
	for _, test := range []struct {
		line string
		busy bool
	}{
		{"0: 00000000:1FBB 00000000:0000 0A", true},
		{"0: 00000000000000000000000000000000:1FBB 00000000000000000000000000000000:0000 0A", true},
		{"0: 0100007F:1FBB 00000000:0000 01", false},
		{"0: 00000000:0050 00000000:0000 0A", false},
	} {
		busy, err := haPortListening([]byte(header + test.line))
		if err != nil || busy != test.busy {
			t.Fatalf("%s: %t %v", test.line, busy, err)
		}
	}
	if _, err := haPortListening(nil); err == nil {
		t.Fatal("accepted missing TCP table")
	}
}

func TestHomeAssistantRejectsDesktopAndInitialHealthFailure(t *testing.T) {
	_, _ = haTestEnvironment(t)
	t.Setenv("HA_FAKE_FAIL", "desktop")
	if _, _, err := installHomeAssistant(context.Background(), haImage("a"), firstModuleProgress(nil)); err == nil || !strings.Contains(err.Error(), "Docker Desktop") {
		t.Fatalf("desktop accepted: %v", err)
	}
	t.Setenv("HA_FAKE_FAIL", "initial_health")
	if _, _, err := installHomeAssistant(context.Background(), haImage("a"), firstModuleProgress(nil)); err == nil {
		t.Fatal("unhealthy install succeeded")
	}
	data, err := os.ReadFile(os.Getenv("HA_FAKE_STATE"))
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Containers map[string]any `json:"containers"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Containers) != 0 {
		t.Fatal("failed initial container not cleaned up")
	}
	if _, err := haReadState(); err != nil {
		t.Fatal("failed install lost configuration intent")
	}
}
