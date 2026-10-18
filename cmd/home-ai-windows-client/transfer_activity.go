package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Transfer activity is per sync-config file, so an agent and its settings UI
// share the same data even when --config points to a non-default location.
// These files contain paths and statuses only; never credentials or tokens.
type transferActivity struct {
	At            time.Time `json:"at"`
	ProfileID     string    `json:"profile_id,omitempty"`
	Path          string    `json:"path,omitempty"`
	Status        string    `json:"status"`
	ItemsDone     int       `json:"items_done,omitempty"`
	ItemsTotal    int       `json:"items_total,omitempty"`
	UploadedBytes int64     `json:"uploaded_bytes,omitempty"`
	TotalBytes    int64     `json:"total_bytes,omitempty"`
	Error         string    `json:"error,omitempty"`
}

const transferHistoryLimit = 4 << 20

var transferActivityMu sync.Mutex

func transferActivityPaths(configPath string) (current, history string) {
	return configPath + ".transfer-progress.json", configPath + ".transfer-history.jsonl"
}

func safeActivityText(value string, max int) string {
	value = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' {
			return ' '
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > max {
		return string(runes[:max]) + "…"
	}
	return value
}

func saveTransferProgress(configPath string, item transferActivity) error {
	transferActivityMu.Lock()
	defer transferActivityMu.Unlock()
	current, _ := transferActivityPaths(configPath)
	item.At = time.Now().UTC()
	item.Path = safeActivityText(item.Path, 512)
	item.Error = safeActivityText(item.Error, 512)
	if err := os.MkdirAll(filepath.Dir(current), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(current), ".home-ai-progress-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err = temporary.Chmod(0o600); err == nil {
		err = json.NewEncoder(temporary).Encode(item)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporary.Name(), current)
}

func appendTransferHistory(configPath string, item transferActivity) error {
	transferActivityMu.Lock()
	defer transferActivityMu.Unlock()
	_, history := transferActivityPaths(configPath)
	item.At = time.Now().UTC()
	item.Path = safeActivityText(item.Path, 512)
	item.Error = safeActivityText(item.Error, 512)
	if err := os.MkdirAll(filepath.Dir(history), 0o700); err != nil {
		return err
	}
	if info, err := os.Stat(history); err == nil && info.Size() >= transferHistoryLimit {
		// Retain one previous segment. Rotation happens only at event boundaries.
		_ = os.Remove(history + ".1")
		if err := os.Rename(history, history+".1"); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(history, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	return errors.Join(writeErr, f.Close())
}

func readTransferProgress(configPath string) (transferActivity, error) {
	current, _ := transferActivityPaths(configPath)
	f, err := os.Open(current)
	if err != nil {
		return transferActivity{}, err
	}
	defer f.Close()
	var item transferActivity
	err = json.NewDecoder(f).Decode(&item)
	return item, err
}

func readTransferHistory(configPath string, maxRecords int) ([]transferActivity, error) {
	if maxRecords <= 0 || maxRecords > 200 {
		maxRecords = 50
	}
	_, history := transferActivityPaths(configPath)
	records := make([]transferActivity, 0, maxRecords)
	for _, filename := range []string{history + ".1", history} {
		f, err := os.Open(filename)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return records, err
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 4096), 16<<10)
		for scanner.Scan() {
			var item transferActivity
			if json.Unmarshal(scanner.Bytes(), &item) != nil {
				continue // Ignore a torn final record after a process is killed.
			}
			records = append(records, item)
			if len(records) > maxRecords {
				records = records[1:]
			}
		}
		err = errors.Join(scanner.Err(), f.Close())
		if err != nil {
			return records, err
		}
	}
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records, nil
}
