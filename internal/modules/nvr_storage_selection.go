package modules

import (
	"context"
	"errors"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type nvrVolume struct {
	Device string
	UUID   string
}

type nvrVolumeContextKey struct{}

func withNVRStorage(ctx context.Context, selection nvrVolume) context.Context {
	return context.WithValue(ctx, nvrVolumeContextKey{}, selection)
}

func nvrSelectionFromContext(ctx context.Context) nvrVolume {
	value, _ := ctx.Value(nvrVolumeContextKey{}).(nvrVolume)
	return value
}

// A selected NVR purpose is an explicit storage reservation. Do not attach
// an arbitrary host path from a module manifest or from a browser request.
func chooseNVRVolume(ctx context.Context, store Store) (nvrVolume, error) {
	provider, ok := store.(interface {
		ListStoragePurposes(context.Context) ([]state.StoragePurposeRecord, error)
	})
	if !ok {
		return nvrVolume{}, nil // in-memory legacy stores, e.g. unit tests
	}
	records, err := provider.ListStoragePurposes(ctx)
	if err != nil {
		return nvrVolume{}, err
	}
	var selected nvrVolume
	for _, record := range records {
		if record.Purpose != state.StoragePurposeNVR {
			continue
		}
		if selected.Device != "" {
			return nvrVolume{}, errors.New("multiple NVR recording disks were assigned; select exactly one")
		}
		if record.FilesystemUUID == "" {
			return nvrVolume{}, errors.New("NVR storage disk must be formatted and have a filesystem UUID")
		}
		selected = nvrVolume{Device: record.DevicePath, UUID: record.FilesystemUUID}
	}
	return selected, nil
}
