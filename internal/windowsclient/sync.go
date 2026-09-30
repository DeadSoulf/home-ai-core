package windowsclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	SyncConflictStop           = "stop"
	SyncConflictSkip           = "skip"
	SyncConflictReplaceToTrash = "replace-to-trash"
)

type SyncOptions struct {
	ConflictPolicy string
	Upload         UploadOptions
	Progress       func(SyncProgress)
}

type SyncProgress struct {
	Index  int
	Total  int
	Path   string
	Kind   string
	Status string
}

type SyncSummary struct {
	Planned   int
	Copied    int
	Unchanged int
	Skipped   int
	Conflicts int
	Replaced  int
}

func NormalizeSyncConflictPolicy(raw string) (string, error) {
	policy := strings.ToLower(strings.TrimSpace(raw))
	if policy == "" {
		policy = SyncConflictStop
	}
	switch policy {
	case SyncConflictStop, SyncConflictSkip, SyncConflictReplaceToTrash:
		return policy, nil
	default:
		return "", fmt.Errorf(
			"invalid conflict policy %q; use %s, %s or %s",
			raw,
			SyncConflictStop,
			SyncConflictSkip,
			SyncConflictReplaceToTrash,
		)
	}
}

// SyncOnce performs one additive local-to-Home-AI scan. New and changed local
// items are considered, but local deletions never delete remote data. A remote
// mismatch is handled only by the explicitly selected conflict policy.
func (c *Client) SyncOnce(
	ctx context.Context,
	folderID string,
	source string,
	destination string,
	options SyncOptions,
) (SyncSummary, error) {
	var summary SyncSummary
	if c == nil || c.BaseURL == nil {
		return summary, errors.New("sync requires a client with a server URL")
	}
	policy, err := NormalizeSyncConflictPolicy(options.ConflictPolicy)
	if err != nil {
		return summary, err
	}
	transfers, err := PlanCopy(source, destination)
	if err != nil {
		return summary, err
	}
	summary.Planned = len(transfers)
	for index, transfer := range transfers {
		if err := ctx.Err(); err != nil {
			return summary, err
		}

		_, exists, verifyErr := c.VerifyTransfer(ctx, folderID, transfer)
		if verifyErr == nil && exists {
			summary.Unchanged++
			emitSyncProgress(options.Progress, index, len(transfers), transfer, "unchanged")
			continue
		}
		if verifyErr != nil {
			var conflict *DestinationConflictError
			if !errors.As(verifyErr, &conflict) {
				return summary, fmt.Errorf("inspect %s: %w", transfer.Destination, verifyErr)
			}
			summary.Conflicts++
			emitSyncProgress(options.Progress, index, len(transfers), transfer, "conflict")
			switch policy {
			case SyncConflictStop:
				return summary, conflict
			case SyncConflictSkip:
				summary.Skipped++
				emitSyncProgress(options.Progress, index, len(transfers), transfer, "skipped")
				continue
			case SyncConflictReplaceToTrash:
				// Replacing an ancestor could discard unrelated data. A planned
				// directory transfer will encounter its own exact conflict first,
				// so only the exact destination is eligible for automatic trash.
				if conflict.Path != transfer.Destination {
					return summary, fmt.Errorf(
						"refusing to replace conflicting ancestor %s while syncing %s",
						conflict.Path,
						transfer.Destination,
					)
				}
				if err := c.trashSyncEntry(ctx, folderID, transfer.Destination); err != nil {
					return summary, fmt.Errorf("move conflicting destination %s to trash: %w", transfer.Destination, err)
				}
				summary.Replaced++
				emitSyncProgress(options.Progress, index, len(transfers), transfer, "trashed")
			}
		}

		if _, err := c.CopyTransfer(ctx, folderID, transfer, options.Upload); err != nil {
			return summary, fmt.Errorf("sync %s: %w", transfer.Destination, err)
		}
		summary.Copied++
		emitSyncProgress(options.Progress, index, len(transfers), transfer, "copied")
	}
	return summary, nil
}

func emitSyncProgress(callback func(SyncProgress), index, total int, transfer Transfer, status string) {
	if callback == nil {
		return
	}
	callback(SyncProgress{
		Index:  index + 1,
		Total:  total,
		Path:   transfer.Destination,
		Kind:   transfer.Kind,
		Status: status,
	})
}

func (c *Client) trashSyncEntry(ctx context.Context, folderID, destination string) error {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return errors.New("folder ID is required")
	}
	destination, err := normalizeCopyDestination(destination)
	if err != nil {
		return err
	}
	endpoint := "/api/v1/files/folders/" + url.PathEscape(folderID) +
		"/entry?path=" + url.QueryEscape(destination)
	if err := c.doJSON(ctx, http.MethodDelete, endpoint, nil, nil, true); err != nil {
		return err
	}
	return nil
}
