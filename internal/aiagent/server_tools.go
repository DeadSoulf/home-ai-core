package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	homenetwork "github.com/DeadSoulf/home-ai-core/internal/network"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
)

type networkProfileSaveInput struct {
	Interface string   `json:"interface"`
	Method    string   `json:"method"`
	Address   string   `json:"address,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	DNS       []string `json:"dns,omitempty"`
}

func (s *Service) networkProfiles(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if err := decodeEmptyObject(input); err != nil {
		return nil, err
	}
	status, err := homenetwork.InspectNetworkProfiles(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(status)
}

func (s *Service) wireGuardStatus(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if err := decodeEmptyObject(input); err != nil {
		return nil, err
	}
	status, err := homenetwork.InspectWireGuard(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(status)
}

func (s *Service) networkProfileSave(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var request networkProfileSaveInput
	if err := decodeToolInput(input, &request); err != nil {
		return nil, err
	}
	request.Interface = strings.TrimSpace(request.Interface)
	request.Method = strings.ToLower(strings.TrimSpace(request.Method))
	if request.Interface == "" || (request.Method != "dhcp" && request.Method != "static") {
		return nil, ErrInvalidToolInput
	}
	if request.Method == "static" && strings.TrimSpace(request.Address) == "" {
		return nil, ErrInvalidToolInput
	}
	message, err := homenetwork.Execute(ctx, homenetwork.Request{
		Operation:     "profile.save",
		Interface:     request.Interface,
		Address:       request.Address,
		Gateway:       request.Gateway,
		NetworkMethod: request.Method,
		DNS:           request.DNS,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"message": message})
}

type networkLinkSetInput struct {
	Interface string `json:"interface"`
	State     string `json:"state"`
}

func (s *Service) networkLinkSet(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var request networkLinkSetInput
	if err := decodeToolInput(input, &request); err != nil {
		return nil, err
	}
	request.Interface = strings.TrimSpace(request.Interface)
	request.State = strings.ToLower(strings.TrimSpace(request.State))
	if request.Interface == "" || (request.State != "up" && request.State != "down") {
		return nil, ErrInvalidToolInput
	}
	message, err := homenetwork.Execute(ctx, homenetwork.Request{
		Operation: "link." + request.State,
		Interface: request.Interface,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"message": message})
}

func (s *Service) storageInspect(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if err := decodeEmptyObject(input); err != nil {
		return nil, err
	}
	inspection, err := storage.Inspect(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(inspection)
}

type storageMountInput struct {
	Device     string `json:"device"`
	Mountpoint string `json:"mountpoint,omitempty"`
}

func (s *Service) storageMount(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var request storageMountInput
	if err := decodeToolInput(input, &request); err != nil {
		return nil, err
	}
	request.Device = strings.TrimSpace(request.Device)
	request.Mountpoint = strings.TrimSpace(request.Mountpoint)
	if !strings.HasPrefix(request.Device, "/dev/") {
		return nil, ErrInvalidToolInput
	}
	if request.Mountpoint != "" {
		clean := filepath.Clean(request.Mountpoint)
		base := filepath.Clean("/mnt/home-ai-core")
		if clean != base && !strings.HasPrefix(clean, base+string(filepath.Separator)) {
			return nil, errors.New("AI storage mountpoint must be under /mnt/home-ai-core")
		}
		request.Mountpoint = clean
	}
	message, err := storage.Execute(ctx, storage.Request{
		Operation:  "mount",
		Device:     request.Device,
		Mountpoint: request.Mountpoint,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"message": message, "mountpoint": request.Mountpoint})
}

type storageUnmountInput struct {
	Device string `json:"device"`
}

func (s *Service) storageUnmount(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var request storageUnmountInput
	if err := decodeToolInput(input, &request); err != nil {
		return nil, err
	}
	request.Device = strings.TrimSpace(request.Device)
	if !strings.HasPrefix(request.Device, "/dev/") {
		return nil, ErrInvalidToolInput
	}
	message, err := storage.Execute(ctx, storage.Request{
		Operation: "unmount",
		Device:    request.Device,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"message": message})
}
