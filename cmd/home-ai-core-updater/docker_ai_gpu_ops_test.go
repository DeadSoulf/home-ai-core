package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestAIGPUValidatePCISet(t *testing.T) {
	good := []string{"0000:01:00.0", "0000:02:00.0"}
	got, err := validateAIGPUPCI(good)
	if err != nil || !reflect.DeepEqual(got, good) {
		t.Fatalf("two distinct GPUs rejected: %#v %v", got, err)
	}
	if got, err := validateAIGPUPCI(nil); err != nil || len(got) != 0 {
		t.Fatalf("CPU fallback rejected: %#v %v", got, err)
	}
	for _, list := range [][]string{
		{"../../etc/passwd"},
		{"0000:01:00a0"},
		{"0000:01:00.0", "0000:01:00.0"},
		{"0000:01:00.0", "0000:02:00.1", "0000:03:00.0",
			"0000:04:00.0", "0000:05:00.0", "0000:06:00.0",
			"0000:07:00.0", "0000:08:00.0", "0000:09:00.0"},
	} {
		if _, err := validateAIGPUPCI(list); err == nil {
			t.Fatalf("unsafe GPU set accepted: %v", list)
		}
	}
}

func TestAINvidiaBusNormalization(t *testing.T) {
	for input, expected := range map[string]string{
		"00000000:01:00.0": "0000:01:00.0",
		"0000:02:00.0":     "0000:02:00.0",
		"FFFF:FF:1F.7":     "ffff:ff:1f.7",
		"127.0.0.1":        "",
		"0000:01:00.9":     "",
	} {
		if got := normalizeNvidiaBusID(input); got != expected {
			t.Errorf("normalize %q = %q, want %q", input, got, expected)
		}
	}
}

func TestAISelectedNvidiaGPUsAreMappedToDistinctStableUUIDs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell stub")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "nvidia-smi")
	const gpuA = "GPU-11111111-1111-1111-1111-111111111111"
	const gpuB = "GPU-22222222-2222-2222-2222-222222222222"
	script := "#!/bin/sh\nprintf '00000000:01:00.0, " + gpuA +
		"\\n00000000:02:00.0, " + gpuB + "\\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ids, err := mapAIGPUsToUUIDs(context.Background(), []string{"0000:02:00.0", "0000:01:00.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []string{gpuB, gpuA}) {
		t.Fatalf("invalid UUID assignment: %v", ids)
	}
	if _, err := mapAIGPUsToUUIDs(context.Background(), []string{"0000:03:00.0"}); err == nil {
		t.Fatal("GPU not in driver was accepted")
	}
	if strings.Contains(strings.Join(ids, ","), "--gpus") {
		t.Fatal("unexpected Docker option in UUIDs")
	}
}
