package security

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

const (
	ProfileAdministrator = "administrator"
	ProfileParent        = "parent"
	ProfileChild         = "child"
	ProfileGuest         = "guest"
	ProfileFriend        = "friend"
)

var (
	ErrLastAdministrator     = errors.New("at least one enabled administrator must remain")
	ErrAdministratorRequired = errors.New("administrator profile is required to manage user access")
)

type ProfileTemplate struct {
	ID                 string   `json:"id"`
	Description        string   `json:"description"`
	FullAccess         bool     `json:"full_access"`
	DefaultPermissions []string `json:"default_permissions"`
}

type PermissionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type ResourceDefinition struct {
	Type        string   `json:"type"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions"`
	OwnerUserID string   `json:"owner_user_id,omitempty"`
}

type AccessCatalog struct {
	Profiles    []ProfileTemplate      `json:"profiles"`
	Permissions []PermissionDefinition `json:"permissions"`
	Resources   []ResourceDefinition   `json:"resources"`
}

type UserAccessInput struct {
	Profile             string
	Permissions         []string
	ResourcePermissions []PermissionScope
	Disabled            bool
}

func NormalizeProfile(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case ProfileAdministrator, ProfileParent, ProfileChild, ProfileGuest, ProfileFriend:
		return value, nil
	default:
		return "", fmt.Errorf("profile must be administrator, parent, child, guest or friend")
	}
}

func profileRoleID(profile string) (string, error) {
	switch profile {
	case ProfileAdministrator:
		return "role_owner", nil
	case ProfileParent:
		return "role_parent", nil
	case ProfileChild:
		return "role_child", nil
	case ProfileGuest:
		return "role_guest", nil
	case ProfileFriend:
		return "role_member", nil
	default:
		return "", fmt.Errorf("unsupported user profile %q", profile)
	}
}

func profileFromRoles(roles []string) string {
	for _, role := range roles {
		switch role {
		case ProfileAdministrator, "owner":
			return ProfileAdministrator
		case ProfileParent:
			return ProfileParent
		case ProfileChild:
			return ProfileChild
		case ProfileGuest:
			return ProfileGuest
		case ProfileFriend, "member":
			return ProfileFriend
		}
	}
	return ProfileFriend
}

func (s *Service) AccessCatalog(ctx context.Context) (AccessCatalog, error) {
	permissionRecords, err := s.store.ListPermissions(ctx)
	if err != nil {
		return AccessCatalog{}, err
	}
	permissions := make([]PermissionDefinition, 0, len(permissionRecords))
	available := make(map[string]bool, len(permissionRecords))
	for _, record := range permissionRecords {
		category := record.Name
		if prefix, _, ok := strings.Cut(record.Name, "."); ok {
			category = prefix
		}
		permissions = append(permissions, PermissionDefinition{
			Name:        record.Name,
			Description: record.Description,
			Category:    category,
		})
		available[record.Name] = true
	}

	profiles := []ProfileTemplate{
		{
			ID:                 ProfileAdministrator,
			Description:        "Full Home-AI administration",
			FullAccess:         true,
			DefaultPermissions: permissionNames(permissionRecords),
		},
		{
			ID:          ProfileParent,
			Description: "Household parent with safe read access by default",
			DefaultPermissions: filterPermissions(available, []string{
				"system.read",
				"events.read",
				"jobs.read",
				"modules.read",
				"updates.read",
				"storage.read",
				"network.read",
			}),
		},
		{
			ID:          ProfileChild,
			Description: "Child account with limited household visibility",
			DefaultPermissions: filterPermissions(available, []string{
				"system.read",
				"events.read",
			}),
		},
		{
			ID:                 ProfileFriend,
			Description:        "Trusted visitor account with explicitly assigned access",
			DefaultPermissions: []string{},
		},
		{
			ID:                 ProfileGuest,
			Description:        "Minimal temporary account",
			DefaultPermissions: []string{},
		},
	}

	folders, err := s.store.ListNASFolders(ctx)
	if err != nil {
		return AccessCatalog{}, err
	}
	resources := make([]ResourceDefinition, 0, len(folders))
	for _, folder := range folders {
		resources = append(resources, ResourceDefinition{
			Type:        "file_folder",
			ID:          folder.ID,
			Name:        folder.Name,
			Description: folder.PoolName,
			Permissions: []string{"files.read", "files.write"},
			OwnerUserID: folder.OwnerUserID,
		})
	}

	cameras, err := s.store.ListNVRCameras(ctx)
	if err != nil {
		return AccessCatalog{}, err
	}
	for _, camera := range cameras {
		resources = append(resources, ResourceDefinition{
			Type:        "camera",
			ID:          camera.ID,
			Name:        camera.Name,
			Description: camera.SourceType,
			Permissions: []string{
				"camera.live",
				"camera.archive",
				"camera.export",
				"camera.ptz",
				"camera.manage",
			},
		})
	}

	return AccessCatalog{
		Profiles:    profiles,
		Permissions: permissions,
		Resources:   resources,
	}, nil
}

func permissionNames(records []state.PermissionRecord) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		result = append(result, record.Name)
	}
	sort.Strings(result)
	return result
}

func filterPermissions(available map[string]bool, requested []string) []string {
	result := make([]string, 0, len(requested))
	for _, permission := range requested {
		if available[permission] {
			result = append(result, permission)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Service) CreateUserWithAccess(
	ctx context.Context,
	actor Actor,
	username, displayName, password string,
	access UserAccessInput,
	meta RequestContext,
) (User, error) {
	if !actorIsAdministrator(actor) {
		return User{}, ErrAdministratorRequired
	}
	profile, permissions, scopes, err := s.normalizeAccess(ctx, access)
	if err != nil {
		return User{}, err
	}

	username, err = NormalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	displayName, err = NormalizeDisplayName(displayName, username)
	if err != nil {
		return User{}, err
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	userID, err := newID("usr_")
	if err != nil {
		return User{}, err
	}
	roleID, err := profileRoleID(profile)
	if err != nil {
		return User{}, err
	}

	record, err := s.store.CreateUserWithAccess(
		ctx,
		userID,
		username,
		displayName,
		passwordHash,
		roleID,
		permissions,
		toStateScopes(scopes),
		s.now().UTC(),
	)
	if errors.Is(err, state.ErrUserExists) {
		return User{}, ErrUserExists
	}
	if err != nil {
		return User{}, err
	}

	account, err := s.store.UserAccount(ctx, record.ID)
	if err != nil {
		return User{}, err
	}
	user := userFromRecord(account)
	s.audit(
		ctx,
		meta,
		actor,
		"security.user.create",
		"user",
		user.ID,
		"success",
		map[string]any{
			"username":             user.Username,
			"profile":              user.Profile,
			"permissions":          user.Permissions,
			"resource_permissions": user.ResourcePermissions,
		},
	)
	return user, nil
}

func (s *Service) UpdateUserAccess(
	ctx context.Context,
	actor Actor,
	userID string,
	access UserAccessInput,
	meta RequestContext,
) (User, error) {
	if !actorIsAdministrator(actor) {
		return User{}, ErrAdministratorRequired
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return User{}, errors.New("user id is required")
	}
	current, err := s.store.UserAccount(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, sql.ErrNoRows
		}
		return User{}, err
	}
	currentProfile := profileFromRoles(current.Roles)

	profile, permissions, scopes, err := s.normalizeAccess(ctx, access)
	if err != nil {
		return User{}, err
	}

	if actor.ID == userID && currentProfile == ProfileAdministrator && (profile != ProfileAdministrator || access.Disabled) {
		return User{}, errors.New("administrator cannot remove or disable their own administrator access")
	}
	if currentProfile == ProfileAdministrator && (profile != ProfileAdministrator || access.Disabled) {
		count, err := s.store.CountEnabledUsersWithRole(ctx, "role_owner")
		if err != nil {
			return User{}, err
		}
		if count <= 1 {
			return User{}, ErrLastAdministrator
		}
	}

	roleID, err := profileRoleID(profile)
	if err != nil {
		return User{}, err
	}
	if err := s.store.SetUserAccess(
		ctx,
		userID,
		roleID,
		permissions,
		toStateScopes(scopes),
		access.Disabled,
		s.now().UTC(),
	); err != nil {
		if errors.Is(err, state.ErrLastEnabledAdministrator) {
			return User{}, ErrLastAdministrator
		}
		return User{}, err
	}

	updated, err := s.store.UserAccount(ctx, userID)
	if err != nil {
		return User{}, err
	}
	user := userFromRecord(updated)
	s.audit(
		ctx,
		meta,
		actor,
		"security.user.access.update",
		"user",
		user.ID,
		"success",
		map[string]any{
			"profile":              user.Profile,
			"permissions":          user.Permissions,
			"resource_permissions": user.ResourcePermissions,
			"disabled":             user.Disabled,
		},
	)
	return user, nil
}

func (s *Service) normalizeAccess(
	ctx context.Context,
	access UserAccessInput,
) (string, []string, []PermissionScope, error) {
	profile, err := NormalizeProfile(access.Profile)
	if err != nil {
		return "", nil, nil, err
	}
	catalog, err := s.AccessCatalog(ctx)
	if err != nil {
		return "", nil, nil, err
	}
	available := make(map[string]bool, len(catalog.Permissions))
	for _, permission := range catalog.Permissions {
		available[permission.Name] = true
	}

	permissionSet := map[string]bool{}
	for _, permission := range access.Permissions {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			continue
		}
		if !available[permission] {
			return "", nil, nil, fmt.Errorf("unknown permission %q", permission)
		}
		if profile != ProfileAdministrator && administratorOnlyPermission(permission) {
			return "", nil, nil, fmt.Errorf("permission %q requires administrator profile", permission)
		}
		permissionSet[permission] = true
	}
	if permissionSet["files.write"] {
		permissionSet["files.read"] = true
	}
	permissions := make([]string, 0, len(permissionSet))
	for permission := range permissionSet {
		permissions = append(permissions, permission)
	}
	sort.Strings(permissions)

	resourcePermissions := map[string]map[string]bool{}
	for _, resource := range catalog.Resources {
		key := resource.Type + "\x00" + resource.ID
		allowed := map[string]bool{}
		for _, permission := range resource.Permissions {
			allowed[permission] = true
		}
		resourcePermissions[key] = allowed
	}

	scopeSet := map[string]PermissionScope{}
	for _, scope := range access.ResourcePermissions {
		scope.Permission = strings.TrimSpace(scope.Permission)
		scope.ResourceType = strings.TrimSpace(scope.ResourceType)
		scope.ResourceID = strings.TrimSpace(scope.ResourceID)
		if scope.Permission == "" || scope.ResourceType == "" || scope.ResourceID == "" {
			return "", nil, nil, errors.New("resource permission requires permission, resource type and resource id")
		}
		if !available[scope.Permission] {
			return "", nil, nil, fmt.Errorf("unknown permission %q", scope.Permission)
		}
		resourceKey := scope.ResourceType + "\x00" + scope.ResourceID
		allowedPermissions, ok := resourcePermissions[resourceKey]
		if !ok {
			return "", nil, nil, fmt.Errorf("unknown access resource %s/%s", scope.ResourceType, scope.ResourceID)
		}
		if !allowedPermissions[scope.Permission] {
			return "", nil, nil, fmt.Errorf(
				"permission %q is not supported by resource %s/%s",
				scope.Permission,
				scope.ResourceType,
				scope.ResourceID,
			)
		}
		key := scope.Permission + "\x00" + scope.ResourceType + "\x00" + scope.ResourceID
		scopeSet[key] = scope
		if scope.Permission == "files.write" {
			read := scope
			read.Permission = "files.read"
			scopeSet[read.Permission+"\x00"+read.ResourceType+"\x00"+read.ResourceID] = read
		}
	}
	scopes := make([]PermissionScope, 0, len(scopeSet))
	for _, scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].ResourceType != scopes[j].ResourceType {
			return scopes[i].ResourceType < scopes[j].ResourceType
		}
		if scopes[i].ResourceID != scopes[j].ResourceID {
			return scopes[i].ResourceID < scopes[j].ResourceID
		}
		return scopes[i].Permission < scopes[j].Permission
	})

	if profile == ProfileAdministrator {
		permissions = permissionNamesFromDefinitions(catalog.Permissions)
	}
	return profile, permissions, scopes, nil
}

func actorIsAdministrator(actor Actor) bool {
	for _, role := range actor.Roles {
		if role == ProfileAdministrator || role == "owner" {
			return true
		}
	}
	return false
}

func administratorOnlyPermission(permission string) bool {
	switch permission {
	case "security.users.manage", "security.roles.manage", "modules.manage", "nvr.storage.manage", "nvr.settings.manage":
		return true
	default:
		return false
	}
}

func permissionNamesFromDefinitions(records []PermissionDefinition) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		result = append(result, record.Name)
	}
	sort.Strings(result)
	return result
}

func toStateScopes(scopes []PermissionScope) []state.ResourcePermissionRecord {
	result := make([]state.ResourcePermissionRecord, 0, len(scopes))
	for _, scope := range scopes {
		result = append(result, state.ResourcePermissionRecord{
			Permission:   scope.Permission,
			ResourceType: scope.ResourceType,
			ResourceID:   scope.ResourceID,
		})
	}
	return result
}

func defaultAccessForProfile(ctx context.Context, service *Service, profile string) UserAccessInput {
	catalog, err := service.AccessCatalog(ctx)
	if err != nil {
		return UserAccessInput{Profile: profile}
	}
	for _, item := range catalog.Profiles {
		if item.ID == profile {
			return UserAccessInput{
				Profile:     profile,
				Permissions: append([]string(nil), item.DefaultPermissions...),
			}
		}
	}
	return UserAccessInput{Profile: profile}
}
