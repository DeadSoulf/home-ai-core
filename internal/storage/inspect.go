package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

type inspectionCache struct {
	sync.Mutex
	stats     []updaterhelper.FilesystemStat
	expiresAt time.Time
}

var filesystemInspection inspectionCache

func InspectFilesystems(ctx context.Context) ([]updaterhelper.FilesystemStat, error) {
	now := time.Now()
	filesystemInspection.Lock()
	if now.Before(filesystemInspection.expiresAt) {
		result := append([]updaterhelper.FilesystemStat(nil), filesystemInspection.stats...)
		filesystemInspection.Unlock()
		return result, nil
	}
	filesystemInspection.Unlock()

	if err := ensureCompatibleHelper(ctx); err != nil {
		return nil, err
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", helperSocketPath)
	if err != nil {
		return nil, fmt.Errorf("connect storage helper: %w", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{
		Operation:       "storage.inspect",
		ProtocolVersion: updaterhelper.ProtocolVersion,
	}); err != nil {
		return nil, fmt.Errorf("request filesystem statistics: %w", err)
	}

	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return nil, fmt.Errorf("read filesystem statistics: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "storage helper rejected inspection"
		}
		return nil, fmt.Errorf("inspect filesystems: %s", response.Error)
	}

	result := append([]updaterhelper.FilesystemStat(nil), response.FilesystemStats...)
	filesystemInspection.Lock()
	filesystemInspection.stats = append([]updaterhelper.FilesystemStat(nil), result...)
	filesystemInspection.expiresAt = time.Now().Add(30 * time.Second)
	filesystemInspection.Unlock()
	return result, nil
}

func InvalidateInspectionCache() {
	filesystemInspection.Lock()
	filesystemInspection.stats = nil
	filesystemInspection.expiresAt = time.Time{}
	filesystemInspection.Unlock()
}
