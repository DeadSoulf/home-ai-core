package nvr

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRuntimeExecutablesCanAppearAfterServiceStart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runtime executable lookup test uses Unix executable bits")
	}

	t.Setenv("PATH", "")
	prober := NewFFProbe()
	live := NewFFmpegMJPEGSource()
	recorder := NewFFmpegSegmentRecorder()
	if prober.Available() || live.Available() || recorder.Available() {
		t.Fatal("runtime unexpectedly available with empty PATH")
	}

	dir := t.TempDir()
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)

	if !prober.Available() {
		t.Fatal("ffprobe did not become available after PATH changed")
	}
	if !live.Available() {
		t.Fatal("live ffmpeg did not become available after PATH changed")
	}
	if !recorder.Available() {
		t.Fatal("recording ffmpeg did not become available after PATH changed")
	}
}
