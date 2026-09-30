package api

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unicode"

	"github.com/DeadSoulf/home-ai-core/internal/filedata"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/smb"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

const defaultSMBWorkgroup = "WORKGROUP"

type smbUserResponse struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	SMBUsername string `json:"smb_username"`
	Configured  bool   `json:"configured"`
}

type smbShareResponse struct {
	FolderID   string `json:"folder_id"`
	FolderName string `json:"folder_name"`
	Kind       string `json:"kind"`
	ShareName  string `json:"share_name"`
	UNC        string `json:"unc"`
}

type smbModel struct {
	Users      []security.User
	UserNames  map[string]string
	Shares     []smb.Share
	ShareViews []smbShareResponse
}

func (s *server) smbStatus(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	model, err := s.buildSMBModel(r)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "smb_model_unavailable", err.Error(), nil)
		return
	}

	smbUsers := make([]string, 0, len(model.Users))
	for _, user := range model.Users {
		smbUsers = append(smbUsers, model.UserNames[user.ID])
	}
	status, err := smb.Inspect(r.Context(), smbUsers)
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "smb_status_unavailable", err.Error(), nil)
		return
	}
	configured := map[string]struct{}{}
	for _, name := range status.ConfiguredUsers {
		configured[name] = struct{}{}
	}

	users := make([]smbUserResponse, 0, len(model.Users))
	for _, user := range model.Users {
		name := model.UserNames[user.ID]
		_, ok := configured[name]
		users = append(users, smbUserResponse{
			UserID:      user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			SMBUsername: name,
			Configured:  ok,
		})
	}

	hostname, _ := os.Hostname()
	writeJSON(w, http.StatusOK, map[string]any{
		"smb": map[string]any{
			"available": status.Available,
			"active":    status.Active,
			"error":     status.Error,
			"hostname":  hostname,
			"workgroup": defaultSMBWorkgroup,
			"users":     users,
			"shares":    model.ShareViews,
		},
	})
}

func (s *server) smbOperation(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}

	var input struct {
		Operation string `json:"operation"`
		UserID    string `json:"user_id,omitempty"`
		Password  string `json:"password,omitempty"`
		Workgroup string `json:"workgroup,omitempty"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_smb_request", err.Error(), nil)
		return
	}
	input.Operation = strings.TrimSpace(input.Operation)

	var (
		message string
		err     error
		target  string
		meta    map[string]any
	)
	switch input.Operation {
	case "install":
		message, err = smb.Install(r.Context())
		target = "samba"
	case "set_password":
		model, modelErr := s.buildSMBModel(r)
		if modelErr != nil {
			err = modelErr
			break
		}
		var selected *security.User
		for i := range model.Users {
			if model.Users[i].ID == strings.TrimSpace(input.UserID) {
				selected = &model.Users[i]
				break
			}
		}
		if selected == nil {
			err = errors.New("SMB user not found")
			break
		}
		if len(input.Password) < 12 || len(input.Password) > 256 {
			err = errors.New("SMB password must contain 12 to 256 characters")
			break
		}
		smbUser := model.UserNames[selected.ID]
		message, err = smb.SetPassword(r.Context(), smbUser, input.Password)
		target = selected.ID
		meta = map[string]any{"smb_username": smbUser}
	case "apply":
		model, modelErr := s.buildSMBModel(r)
		if modelErr != nil {
			err = modelErr
			break
		}
		workgroup := strings.TrimSpace(input.Workgroup)
		if workgroup == "" {
			workgroup = defaultSMBWorkgroup
		}
		message, err = smb.Apply(r.Context(), workgroup, model.Shares)
		target = "samba"
		meta = map[string]any{"workgroup": workgroup, "share_count": len(model.Shares)}
	default:
		writeAPIError(w, r, http.StatusBadRequest, "invalid_smb_operation", "unsupported SMB operation", nil)
		return
	}
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "smb_operation_failed", err.Error(), nil)
		return
	}

	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"files.smb."+input.Operation,
		"smb",
		target,
		"success",
		meta,
	)
	s.realtime.Publish(
		"files.smb.changed",
		map[string]any{"operation": input.Operation},
		requestIDFromContext(r.Context()),
	)
	writeJSON(w, http.StatusOK, map[string]any{"message": message})
}

func (s *server) buildSMBModel(r *http.Request) (smbModel, error) {
	users, err := s.security.ListUsers(r.Context())
	if err != nil {
		return smbModel{}, fmt.Errorf("list users for SMB: %w", err)
	}
	folders, err := s.state.ListNASFolders(r.Context())
	if err != nil {
		return smbModel{}, fmt.Errorf("list folders for SMB: %w", err)
	}

	activeUsers := make([]security.User, 0, len(users))
	userNames := map[string]string{}
	ownerIDs := map[string]struct{}{}
	for _, user := range users {
		if user.Disabled {
			continue
		}
		activeUsers = append(activeUsers, user)
		userNames[user.ID] = smbSystemUsername(user.ID)
		for _, role := range user.Roles {
			if role == "owner" {
				ownerIDs[user.ID] = struct{}{}
				break
			}
		}
	}

	hostname, _ := os.Hostname()
	shares := make([]smb.Share, 0, len(folders))
	views := make([]smbShareResponse, 0, len(folders))
	for _, folder := range folders {
		root, err := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath)
		if err != nil {
			return smbModel{}, fmt.Errorf("resolve SMB folder %s: %w", folder.ID, err)
		}

		allowed := map[string]struct{}{}
		for ownerID := range ownerIDs {
			if name := userNames[ownerID]; name != "" {
				allowed[name] = struct{}{}
			}
		}
		if folder.Kind == "private" {
			if name := userNames[folder.OwnerUserID]; name != "" {
				allowed[name] = struct{}{}
			}
		} else {
			for _, user := range activeUsers {
				if name := userNames[user.ID]; name != "" {
					allowed[name] = struct{}{}
				}
			}
		}

		writeUsers := make([]string, 0, len(allowed))
		for name := range allowed {
			writeUsers = append(writeUsers, name)
		}
		if len(writeUsers) == 0 {
			continue
		}
		shareName := smbShareName(folder)
		shares = append(shares, smb.Share{
			Name:       shareName,
			Path:       root,
			WriteUsers: writeUsers,
		})
		views = append(views, smbShareResponse{
			FolderID:   folder.ID,
			FolderName: folder.Name,
			Kind:       folder.Kind,
			ShareName:  shareName,
			UNC:        fmt.Sprintf(`\\%s\%s`, hostname, shareName),
		})
	}

	return smbModel{
		Users:      activeUsers,
		UserNames:  userNames,
		Shares:     shares,
		ShareViews: views,
	}, nil
}

func smbSystemUsername(userID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(userID)))
	return "hai_" + hex.EncodeToString(sum[:10])
}

func smbShareName(folder state.NASFolderRecord) string {
	var b strings.Builder
	for _, r := range folder.Name {
		switch {
		case unicode.IsLetter(r) && r <= unicode.MaxASCII:
			b.WriteRune(r)
		case unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
				b.WriteByte('_')
			}
		}
		if b.Len() >= 36 {
			break
		}
	}
	base := strings.Trim(b.String(), "_-")
	if base == "" {
		base = "Folder"
	}
	suffix := folder.ID
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	return "HA_" + base + "_" + suffix
}
