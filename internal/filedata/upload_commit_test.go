package filedata

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallUploadPartConcurrentDestination(t *testing.T) {
	installers := []struct {
		name    string
		install func(string, string) error
	}{
		{"native", installUploadPart},
		{"hard_link", installUploadPartByLink},
	}
	for _, installer := range installers {
		t.Run(installer.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "target.bin")
			const competitors = 16
			parts := make([]string, competitors)
			payloads := make([][]byte, competitors)
			for i := range parts {
				parts[i] = filepath.Join(root, fmt.Sprintf("part-%d", i))
				payloads[i] = []byte(fmt.Sprintf("payload from competitor %d", i))
				if err := os.WriteFile(parts[i], payloads[i], 0o640); err != nil {
					t.Fatal(err)
				}
			}
			type attempt struct {
				index int
				err   error
			}
			start := make(chan struct{})
			results := make(chan attempt, competitors)
			for i := range parts {
				go func(index int) {
					<-start
					results <- attempt{index, installer.install(parts[index], target)}
				}(i)
			}
			close(start)
			winner := -1
			for range competitors {
				result := <-results
				if result.err == nil {
					if winner != -1 {
						t.Errorf("competitors %d and %d both committed", winner, result.index)
					}
					winner = result.index
					continue
				}
				if !errors.Is(result.err, os.ErrExist) {
					t.Errorf("competitor %d error = %v, want destination exists", result.index, result.err)
				}
				data, err := os.ReadFile(parts[result.index])
				if err != nil || !bytes.Equal(data, payloads[result.index]) {
					t.Errorf("loser %d data = %q, error = %v", result.index, data, err)
				}
			}
			if winner == -1 {
				t.Fatal("no competitor committed")
			}
			data, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(data, payloads[winner]) {
				t.Fatalf("destination data = %q, winner = %d, error = %v", data, winner, err)
			}
			if _, err := os.Lstat(parts[winner]); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("committed part still exists: %v", err)
			}
		})
	}
}

func TestInstallUploadPartPreservesExistingEntries(t *testing.T) {
	installers := []struct {
		name    string
		install func(string, string) error
	}{
		{"native", installUploadPart},
		{"hard_link", installUploadPartByLink},
	}
	for _, installer := range installers {
		for _, kind := range []string{"file", "directory", "symlink"} {
			t.Run(installer.name+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				target := filepath.Join(root, "target")
				createUploadTestTarget(t, kind, target)
				before, err := os.Lstat(target)
				if err != nil {
					t.Fatal(err)
				}
				part := filepath.Join(root, "part")
				payload := []byte("incoming upload")
				if err := os.WriteFile(part, payload, 0o640); err != nil {
					t.Fatal(err)
				}
				if err := installer.install(part, target); !errors.Is(err, os.ErrExist) {
					t.Fatalf("install error = %v, want destination exists", err)
				}
				after, err := os.Lstat(target)
				if err != nil || !os.SameFile(before, after) {
					t.Fatalf("existing %s replaced, error = %v", kind, err)
				}
				data, err := os.ReadFile(part)
				if err != nil || !bytes.Equal(data, payload) {
					t.Fatalf("upload part data = %q, error = %v", data, err)
				}
			})
		}
	}
}

func createUploadTestTarget(t *testing.T, kind, target string) {
	t.Helper()
	var err error
	switch kind {
	case "file":
		err = os.WriteFile(target, []byte("keep existing contents"), 0o640)
	case "directory":
		err = os.Mkdir(target, 0o750)
	case "symlink":
		// A dangling symlink still occupies the destination and must not be
		// followed or replaced.
		err = os.Symlink(filepath.Join(filepath.Dir(target), "missing"), target)
	default:
		t.Fatalf("unknown target kind %q", kind)
	}
	if err != nil {
		t.Fatal(err)
	}
}
