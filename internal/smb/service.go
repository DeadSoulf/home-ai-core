package smb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const helperSocketPath = "/run/home-ai-core-updater.sock"

type Share struct {
	Name       string
	Path       string
	ReadUsers  []string
	WriteUsers []string
}

type Status struct {
	Available       bool     `json:"available"`
	Active          bool     `json:"active"`
	Error           string   `json:"error,omitempty"`
	ConfiguredUsers []string `json:"configured_users"`
}

func Inspect(ctx context.Context, users []string) (Status, error) {
	response, err := callHelper(ctx, updaterhelper.Request{
		Operation:       "smb.inspect",
		ProtocolVersion: updaterhelper.ProtocolVersion,
		SMBUsers:        trimStrings(users),
	})
	if err != nil {
		return Status{}, err
	}
	return Status{
		Available:       response.SMBAvailable,
		Active:          response.SMBActive,
		Error:           response.SMBError,
		ConfiguredUsers: append([]string(nil), response.SMBConfiguredUsers...),
	}, nil
}

func Install(ctx context.Context) (string, error) {
	return execute(ctx, updaterhelper.Request{Operation: "smb.install"})
}

func SetPassword(ctx context.Context, username, password string) (string, error) {
	return execute(ctx, updaterhelper.Request{
		Operation:   "smb.user.set_password",
		SMBUser:     strings.TrimSpace(username),
		SMBPassword: password,
	})
}

func Apply(ctx context.Context, workgroup string, shares []Share) (string, error) {
	requestShares := make([]updaterhelper.SMBShareRequest, 0, len(shares))
	for _, share := range shares {
		requestShares = append(requestShares, updaterhelper.SMBShareRequest{
			Name:       strings.TrimSpace(share.Name),
			Path:       strings.TrimSpace(share.Path),
			ReadUsers:  trimStrings(share.ReadUsers),
			WriteUsers: trimStrings(share.WriteUsers),
		})
	}
	return execute(ctx, updaterhelper.Request{
		Operation:    "smb.apply",
		SMBWorkgroup: strings.TrimSpace(workgroup),
		SMBShares:    requestShares,
	})
}

func execute(ctx context.Context, request updaterhelper.Request) (string, error) {
	request.ProtocolVersion = updaterhelper.ProtocolVersion
	response, err := callHelper(ctx, request)
	if err != nil {
		return "", err
	}
	return response.Message, nil
}

func callHelper(ctx context.Context, request updaterhelper.Request) (updaterhelper.Response, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", helperSocketPath)
	if err != nil {
		return updaterhelper.Response{}, fmt.Errorf("connect SMB helper: %w", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return updaterhelper.Response{}, err
	}
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return updaterhelper.Response{}, err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "SMB helper rejected operation"
		}
		return updaterhelper.Response{}, errors.New(response.Error)
	}
	return response, nil
}

func trimStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
