package modulecatalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

const (
	SchemaVersion       = 1
	CatalogID           = "home-ai-core.official"
	Repository          = "DeadSoulf/home-ai-core_modules"
	catalogAPIURL       = "https://api.github.com/repos/DeadSoulf/home-ai-core_modules/contents/catalog.json?ref=main"
	tokenEnvironment    = "HOME_AI_MODULE_CATALOG_TOKEN"
	officialImagePrefix = "ghcr.io/deadsoulf/"
	maxCatalogBytes     = 2 << 20
	cacheTTL            = 5 * time.Minute
)

var ErrTokenRequired = errors.New("GitHub access token is required for the private official module catalog")

type Catalog struct {
	SchemaVersion int                `json:"schema_version"`
	ID            string             `json:"id"`
	GeneratedAt   string             `json:"generated_at"`
	Modules       []modules.Manifest `json:"modules"`
}

type Snapshot struct {
	Catalog     Catalog
	SourceSHA   string
	RefreshedAt time.Time
	FromCache   bool
	Warning     string
}

type Service struct {
	stateDir    string
	coreVersion string
	client      *http.Client

	mu       sync.Mutex
	cached   Snapshot
	cachedAt time.Time
}

func New(stateDir, coreVersion string) *Service {
	return &Service{
		stateDir:    stateDir,
		coreVersion: coreVersion,
		client: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

func (s *Service) Repository() string {
	return Repository
}

func (s *Service) TokenConfigured() bool {
	return strings.TrimSpace(s.token()) != ""
}

func (s *Service) RegistryCredentials() (string, string) {
	return "DeadSoulf", s.token()
}

func (s *Service) SaveToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	token = strings.TrimSpace(token)
	path := s.tokenPath()
	if token == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove module catalog token: %w", err)
		}
		s.cachedAt = time.Time{}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create module catalog state directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".github-token-*")
	if err != nil {
		return fmt.Errorf("create module catalog token file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("protect module catalog token file: %w", err)
	}
	if _, err := temp.WriteString(token + "\n"); err != nil {
		temp.Close()
		return fmt.Errorf("write module catalog token: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close module catalog token file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("activate module catalog token: %w", err)
	}
	s.cachedAt = time.Time{}
	return nil
}

func (s *Service) Load(ctx context.Context, refresh bool) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !refresh && !s.cachedAt.IsZero() && time.Since(s.cachedAt) < cacheTTL {
		return s.cached, nil
	}

	snapshot, err := s.fetch(ctx)
	if err == nil {
		if cacheErr := s.saveCache(snapshot.Catalog); cacheErr != nil {
			snapshot.Warning = cacheErr.Error()
		}
		s.cached = snapshot
		s.cachedAt = time.Now()
		return snapshot, nil
	}

	cached, cacheErr := s.loadCache()
	if cacheErr != nil {
		return Snapshot{}, err
	}
	cached.FromCache = true
	cached.Warning = err.Error()
	s.cached = cached
	s.cachedAt = time.Now()
	return cached, nil
}

func (s *Service) Module(ctx context.Context, id string, refresh bool) (modules.Manifest, Snapshot, error) {
	snapshot, err := s.Load(ctx, refresh)
	if err != nil {
		return modules.Manifest{}, Snapshot{}, err
	}
	for _, manifest := range snapshot.Catalog.Modules {
		if manifest.ID == id {
			return manifest, snapshot, nil
		}
	}
	return modules.Manifest{}, snapshot, fmt.Errorf("module %q is not published in the official catalog", id)
}

func (s *Service) fetch(ctx context.Context) (Snapshot, error) {
	token := strings.TrimSpace(s.token())
	if token == "" {
		return Snapshot{}, ErrTokenRequired
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogAPIURL, nil)
	if err != nil {
		return Snapshot{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "home-ai-core/"+strings.TrimSpace(s.coreVersion))

	resp, err := s.client.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load official module catalog: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogBytes+1))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read official module catalog response: %w", err)
	}
	if len(body) > maxCatalogBytes {
		return Snapshot{}, errors.New("official module catalog response is too large")
	}
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("GitHub module catalog request failed with HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Snapshot{}, fmt.Errorf("decode GitHub module catalog response: %w", err)
	}
	if payload.Encoding != "base64" || strings.TrimSpace(payload.Content) == "" {
		return Snapshot{}, errors.New("GitHub module catalog response does not contain base64 content")
	}
	raw, err := base64.StdEncoding.DecodeString(payload.Content)
	if err != nil {
		return Snapshot{}, fmt.Errorf("decode official module catalog content: %w", err)
	}
	catalog, err := decodeCatalog(raw)
	if err != nil {
		return Snapshot{}, err
	}
	now := time.Now().UTC()
	return Snapshot{
		Catalog:     catalog,
		SourceSHA:   payload.SHA,
		RefreshedAt: now,
	}, nil
}

func decodeCatalog(raw []byte) (Catalog, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()

	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("decode official module catalog: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Catalog{}, errors.New("official module catalog must contain one JSON object")
	}
	if err := validateCatalog(catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func validateCatalog(catalog Catalog) error {
	if catalog.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported module catalog schema version %d", catalog.SchemaVersion)
	}
	if catalog.ID != CatalogID {
		return fmt.Errorf("unexpected module catalog id %q", catalog.ID)
	}
	if _, err := time.Parse(time.RFC3339, catalog.GeneratedAt); err != nil {
		return errors.New("module catalog generated_at must be RFC3339")
	}

	seen := make(map[string]struct{}, len(catalog.Modules))
	for _, manifest := range catalog.Modules {
		if err := modules.ValidateManifest(manifest); err != nil {
			return fmt.Errorf("catalog module %q: %w", manifest.ID, err)
		}
		if _, exists := seen[manifest.ID]; exists {
			return fmt.Errorf("duplicate catalog module %q", manifest.ID)
		}
		seen[manifest.ID] = struct{}{}
		image := strings.ToLower(strings.TrimSpace(manifest.Runtime.Docker.Image))
		if !strings.HasPrefix(image, officialImagePrefix) {
			return fmt.Errorf("catalog module %q uses a non-official Docker image", manifest.ID)
		}
	}
	return nil
}

func (s *Service) token() string {
	if value := strings.TrimSpace(os.Getenv(tokenEnvironment)); value != "" {
		return value
	}
	raw, err := os.ReadFile(s.tokenPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func (s *Service) tokenPath() string {
	return filepath.Join(s.stateDir, "module-catalog", "github-token")
}

func (s *Service) cachePath() string {
	return filepath.Join(s.stateDir, "module-catalog", "catalog.json")
}

func (s *Service) saveCache(catalog Catalog) error {
	path := s.cachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create module catalog cache directory: %w", err)
	}
	raw, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("encode module catalog cache: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write module catalog cache: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect module catalog cache: %w", err)
	}
	return nil
}

func (s *Service) loadCache() (Snapshot, error) {
	path := s.cachePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("official module catalog is unavailable and no cached catalog exists: %w", err)
	}
	catalog, err := decodeCatalog(raw)
	if err != nil {
		return Snapshot{}, fmt.Errorf("cached module catalog is invalid: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Catalog:     catalog,
		RefreshedAt: info.ModTime().UTC(),
		FromCache:   true,
	}, nil
}
