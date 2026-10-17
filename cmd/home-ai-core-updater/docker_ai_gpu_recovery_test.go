package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAIGPUFailedCandidateRestoresOriginalContainerAndData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX Docker stub only")
	}
	temp := t.TempDir()
	oldRoot := moduleDataRoot
	moduleDataRoot = filepath.Join(temp, "modules")
	t.Cleanup(func() { moduleDataRoot = oldRoot })
	data := filepath.Join(moduleDataRoot, "ai.agent")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(data, "keep-this.sqlite")
	if err := os.WriteFile(sentinel, []byte("PERSISTENT AI DATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := map[string]any{
		"State": map[string]any{"Running": true},
		"Config": map[string]any{"Image": "ghcr.io/test/ai@sha256:" + strings.Repeat("a", 64),
			"Labels": map[string]string{"home-ai.managed": "true", "home-ai.module.id": "ai.agent"}},
		"Mounts": []map[string]string{
			{"Type": "bind", "Source": data, "Destination": "/data"},
		},
		"NetworkSettings": map[string]any{"Networks": map[string]any{
			"home-ai-modules": map[string]string{"IPAddress": "172.19.0.6", "NetworkID": "network-id"},
		}},
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(temp, "docker-commands.log")
	docker := "#!/bin/sh\n" +
		"echo \"$*\" >> '" + logPath + "'\n" +
		"case \"$1\" in\n" +
		" network) printf 'true|network-id\\n';;\n" +
		" inspect)\n" +
		"  case \"$4\" in *gpu-candidate|*gpu-previous) echo 'No such object' >&2; exit 1;; esac\n" +
		"  case \"$3\" in\n" +
		"   *json*) printf '%s\\n' '" + string(raw) + "';;\n" +
		"   *Labels*) printf 'true|ai.agent\\n';;\n" +
		"   *) echo 'No such object' >&2; exit 1;;\n" +
		"  esac;;\n" +
		" create) echo 'candidate-image-id';;\n" +
		" start) if [ \"$2\" = 'home-ai-module-ai.agent-gpu-candidate' ]; then echo 'GPU toolkit refused container' >&2; exit 1; fi;;\n" +
		" stop|rm|rename) exit 0;;\n" +
		" *) exit 2;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(temp, "docker"), []byte(docker), 0o755); err != nil {
		t.Fatal(err)
	}
	const uuid = "GPU-11111111-1111-1111-1111-111111111111"
	nvidia := "#!/bin/sh\nprintf '00000000:01:00.0, " + uuid + "\\n'\n"
	if err := os.WriteFile(filepath.Join(temp, "nvidia-smi"), []byte(nvidia), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", temp+string(os.PathListSeparator)+os.Getenv("PATH"))
	assigned, err := applyAIGPUSelection(context.Background(), []string{"0000:01:00.0"}, 1000, 1000)
	if err == nil || assigned != nil {
		t.Fatalf("failed candidate was accepted: %v %v", assigned, err)
	}
	bytes, err := os.ReadFile(sentinel)
	if err != nil || string(bytes) != "PERSISTENT AI DATA" {
		t.Fatalf("persistent AI data was touched: %q %v", bytes, err)
	}
	trace, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(trace)
	for _, command := range []string{
		"stop home-ai-module-ai.agent",
		"start home-ai-module-ai.agent-gpu-candidate",
		"rm -f home-ai-module-ai.agent-gpu-candidate",
		"start home-ai-module-ai.agent\n",
		"--gpus \"device=" + uuid + "\"",
	} {
		if !strings.Contains(log, command) {
			t.Errorf("missing rollback command %q in trace: %s", command, log)
		}
	}
	if strings.Contains(log, "rm -f home-ai-module-ai.agent\n") {
		t.Fatalf("original container was deleted: %s", log)
	}
}
