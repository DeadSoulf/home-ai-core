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

type Inspection struct {
	Filesystems []updaterhelper.FilesystemStat
	DiskHealth  []updaterhelper.DiskHealthStat
	LVM         []updaterhelper.LVMStat
}

type inspectionCache struct {
	sync.Mutex
	value     Inspection
	expiresAt time.Time
}

var storageInspection inspectionCache

func Inspect(ctx context.Context) (Inspection, error) {
	now := time.Now()
	storageInspection.Lock()
	if now.Before(storageInspection.expiresAt) {
		result := cloneInspection(storageInspection.value)
		storageInspection.Unlock()
		return result, nil
	}
	storageInspection.Unlock()

	if err := ensureCompatibleHelper(ctx); err != nil {
		return Inspection{}, err
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", helperSocketPath)
	if err != nil {
		return Inspection{}, fmt.Errorf("connect storage helper: %w", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{
		Operation:       "storage.inspect",
		ProtocolVersion: updaterhelper.ProtocolVersion,
	}); err != nil {
		return Inspection{}, fmt.Errorf("request storage inspection: %w", err)
	}

	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return Inspection{}, fmt.Errorf("read storage inspection: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "storage helper rejected inspection"
		}
		return Inspection{}, fmt.Errorf("inspect storage: %s", response.Error)
	}

	result := Inspection{
		Filesystems: append([]updaterhelper.FilesystemStat(nil), response.FilesystemStats...),
		DiskHealth:  append([]updaterhelper.DiskHealthStat(nil), response.DiskHealth...),
		LVM:         append([]updaterhelper.LVMStat(nil), response.LVM...),
	}
	storageInspection.Lock()
	storageInspection.value = cloneInspection(result)
	storageInspection.expiresAt = time.Now().Add(30 * time.Second)
	storageInspection.Unlock()
	return result, nil
}

func InvalidateInspectionCache() {
	storageInspection.Lock()
	storageInspection.value = Inspection{}
	storageInspection.expiresAt = time.Time{}
	storageInspection.Unlock()
}

func cloneInspection(value Inspection) Inspection {
	return Inspection{
		Filesystems: append([]updaterhelper.FilesystemStat(nil), value.Filesystems...),
		DiskHealth:  append([]updaterhelper.DiskHealthStat(nil), value.DiskHealth...),
		LVM:         append([]updaterhelper.LVMStat(nil), value.LVM...),
	}
}
