package security

import (
	"context"
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
	ProfileFriend        = "friend"
	ProfileGuest         = "guest"
	ProfileMember        = "member"
)

type UserProfileInput struct {
	Username            string
	DisplayName         string
	Password            string
	Profile             string
	Disabled            bool
	Permissions         []string
	ResourcePermissions []PermissionScope
}

type PermissionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ProfileTemplate struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	DefaultPermissions []string `json:"default_permissions"`
	FullAccess         bool     `json:"full_access"`
	Legacy             bool     `json:"legacy,omitempty"`
}

type AccessCatalog struct {
	Permissions []PermissionDefinition `json:"permissions"`
	Profiles    []ProfileTemplate      `json:"profiles"`
}

func (s *Service) AccessCatalog(ctx context.Context) (AccessCatalog, error) {
	records, err := s.store.ListPermissions(ctx)
	if err != nil {
		return AccessCatalog{}, err
	}
	permissions := make([]PermissionDefinition, 0, len(records))
	allNames := make([]string, 0, len(records))
	for _, record := range records {
		permissions = append(permissions, PermissionDefinition{Name: record.Name, Description: record.Description})
		allNames = append(allNames, record.Name)
	}
	sort.Strings(allNames)

	profiles := make([]ProfileTemplate, 0, 6)
	for _, item := range []struct {
		name        string
		description string
		fullAccess  bool
		legacy      bool
	}{
		{ProfileAdministrator, "Full access to every Home-AI capability", true, false},
		{ProfileParent, "Household parent profile with broad read access and no administration by default", false, false},
		{ProfileChild, "Child profile with basic local Home-AI access", false, false},
		{ProfileFriend, "Friend profile with sign-in and explicitly granted resources only", false, false},
		{ProfileGuest, "Guest profile with sign-in and explicitly granted resources only", false, false},
		{ProfileMember, "Legacy household member profile kept for compatibility", false, true},
	} {
		defaults := append([]string(nil), allNames...)
		if !item.fullAccess {
			roleName, _, roleErr := roleForProfile(item.name)
			if roleErr != nil {
				return AccessCatalog{}, roleErr
			}
			defaults, err = s.store.RolePermissions(ctx, roleName)
			if err != nil {
				return AccessCatalog{}, err
			}
		}
		profiles = append(profiles, ProfileTemplate{
			Name: item.name, Description: item.description, DefaultPermissions: defaults,
			FullAccess: item.fullAccess, Legacy: item.legacy,
		})
	}
	return AccessCatalog{Permissions: permissions, Profiles: profiles}, nil
}

func normalizeProfile(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func roleForProfile(profile string) (roleName, roleID string, err error) {
	switch normalizeProfile(profile) {
	case ProfileAdministrator:
		return ProfileAdministrator, "role_administrator", nil
	case ProfileParent:
		return ProfileParent, "role_parent", nil
	case ProfileChild:
		return ProfileChild, "role_child", nil
	case ProfileFriend:
		return ProfileFriend, "role_friend", nil
	case ProfileGuest:
		return ProfileGuest, "role_guest", nil
	case ProfileMember:
		return ProfileMember, "role_member", nil
	default:
		return "", "", ErrInvalidProfile
	}
}

func profileFromRoles(roles []string) string {
	for _, role := range roles {
		switch role {
		case "owner", ProfileAdministrator:
			return ProfileAdministrator
		case ProfileParent:
			return ProfileParent
		case ProfileChild:
			return ProfileChild
		case ProfileFriend:
			return ProfileFriend
		case ProfileGuest:
			return ProfileGuest
		case ProfileMember:
			return ProfileMember
		}
	}
	return ""
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {\n\t\t\treturn true\n\t\t}
	}
	return false
}

func (s *Service) normalizeRequestedPermissions(ctx context.Context, profile string, requested []string) ([]string, error) {
	catalog, err := s.AccessCatalog(ctx)
	if err != nil {\n\t\treturn nil, err\n\t}
	valid := make(map[string]bool, len(catalog.Permissions))
	all := make([]string, 0, len(catalog.Permissions))
	for _, permission := range catalog.Permissions {
		valid[permission.Name] = true
		all = append(all, permission.Name)
	}
	sort.Strings(all)
	if profile == ProfileAdministrator {\n\t\treturn all, nil\n\t}
	if requested == nil {
		for _, template := range catalog.Profiles {
			if template.Name == profile {\n\t\t\t\treturn append([]string(nil), template.DefaultPermissions...), nil\n\t\t\t}
		}
		return nil, ErrInvalidProfile
	}
	resultSet := map[string]bool{}
	for _, permission := range requested {
		permission = strings.TrimSpace(permission)
		if !valid[permission] {\n\t\t\treturn nil, fmt.Errorf("unknown permission %q", permission)\n\t\t}
		resultSet[permission] = true
	}
	result := make([]string, 0, len(resultSet))
	for permission := range resultSet {\n\t\tresult = append(result, permission)\n\t}
	sort.Strings(result)
	return result, nil
}

func (s *Service) normalizeResourcePermissions(ctx context.Context, requested []PermissionScope) ([]PermissionScope, error) {
	catalog, err := s.AccessCatalog(ctx)
	if err != nil { return nil, err }
	valid := make(map[string]bool, len(catalog.Permissions))
	for _, permission := range catalog.Permissions {\n\t\tvalid[permission.Name] = true\n\t}
	result := make([]PermissionScope, 0, len(requested))
	seen := map[string]bool{}
	for _, scope := range requested {
		scope.Permission = strings.TrimSpace(scope.Permission)
		scope.ResourceType = strings.TrimSpace(scope.ResourceType)
		scope.ResourceID = strings.TrimSpace(scope.ResourceID)
		if !valid[scope.Permission] {\n\t\t\treturn nil, fmt.Errorf("unknown scoped permission %q", scope.Permission)\n\t\t}
		if scope.ResourceType == "" || scope.ResourceID == "" {\n\t\t\treturn nil, errors.New("resource permission requires resource type and id")\n\t\t}
		switch scope.ResourceType {
		case "file_folder":
			if scope.Permission != "files.read" && scope.Permission != "files.write" {
				return nil, errors.New("file folder scopes support only files.read and files.write")
			}
		default:
			return nil, fmt.Errorf("unsupported resource type %q", scope.ResourceType)
		}
		key := scope.Permission + "\x00" + scope.ResourceType + "\x00" + scope.ResourceID
		if seen[key] {\n\t\t\tcontinue\n\t\t}
		seen[key] = true
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ResourceType != result[j].ResourceType {\n\t\t\treturn result[i].ResourceType < result[j].ResourceType\n\t\t}
		if result[i].ResourceID != result[j].ResourceID {\n\t\t\treturn result[i].ResourceID < result[j].ResourceID\n\t\t}
		return result[i].Permission < result[j].Permission
	})
	return result, nil
}

func resourcePermissionRecords(scopes []PermissionScope) []state.ResourcePermissionRecord {
	result := make([]state.ResourcePermissionRecord, 0, len(scopes))
	for _, scope := range scopes {
		result = append(result, state.ResourcePermissionRecord{Permission: scope.Permission, ResourceType: scope.ResourceType, ResourceID: scope.ResourceID})
	}
	return result
}
