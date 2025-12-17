package catalog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type BuildpackRegistryClient struct {
	httpClient *http.Client
	cacheDir   string
	force      bool
	limiter    *rateLimiter
}

// rateLimiter implements a token bucket rate limiter
// Rate limit: 10 requests per 5 seconds = 1 request per 500ms
type rateLimiter struct {
	tokens chan struct{}
	ticker *time.Ticker
	once   sync.Once
}

func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{
		tokens: make(chan struct{}, 10),
	}
	for range 10 {
		rl.tokens <- struct{}{}
	}
	rl.ticker = time.NewTicker(500 * time.Millisecond)
	go func() {
		for range rl.ticker.C {
			select {
			case rl.tokens <- struct{}{}:
			default:
			}
		}
	}()
	return rl
}

func (rl *rateLimiter) wait() {
	<-rl.tokens
}

func (rl *rateLimiter) stop() {
	rl.once.Do(func() {
		if rl.ticker != nil {
			rl.ticker.Stop()
		}
	})
}

func NewBuildpackRegistryClient(httpClient *http.Client, cacheDir string, force bool) *BuildpackRegistryClient {
	return &BuildpackRegistryClient{
		httpClient: httpClient,
		cacheDir:   cacheDir,
		force:      force,
		limiter:    newRateLimiter(),
	}
}

func (c *BuildpackRegistryClient) GetBuildpackRegistry(pkg Package) (*registryMetadata, error) {
	cacheFile := filepath.Join(c.cacheDir, fmt.Sprintf("%s-%s-%s.json", pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version()))

	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	fileInfo, err := os.Stat(cacheFile)
	useCache := err == nil && !c.force && fileInfo != nil && fileInfo.Size() > 0

	if useCache {
		content, err := os.ReadFile(cacheFile)
		if err != nil {
			return nil, err
		}

		metadata := &registryMetadata{}
		err = json.Unmarshal(content, metadata)
		if err != nil {
			return nil, err
		}
		return metadata, nil
	}

	url := fmt.Sprintf("https://cnb-registry-api.herokuapp.com/api/v1/buildpacks/%s/%s/%s", pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version())

	const maxRetries = 5
	var response *http.Response

	for attempt := range maxRetries {
		// Wait for rate limiter before making the API request
		c.limiter.wait()

		response, err = c.httpClient.Get(url)
		if err != nil {
			return nil, err
		}

		if response.StatusCode == http.StatusTooManyRequests {
			response.Body.Close()
			if attempt < maxRetries-1 {
				time.Sleep(1 * time.Second)
				continue
			}
			// If this was the last attempt, return error
			return nil, fmt.Errorf("registry API returned status 429 (rate limited) after %d retries for %s/%s/%s", maxRetries, pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version())
		}

		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, fmt.Errorf("registry API returned status %d for %s/%s/%s", response.StatusCode, pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version())
		}

		break
	}
	defer response.Body.Close()

	metadata := &registryMetadata{}
	err = json.NewDecoder(response.Body).Decode(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to decode registry response: %w", err)
	}

	// Validate that we got meaningful data before caching
	if metadata.ID == "" || metadata.Name == "" {
		return nil, fmt.Errorf("registry API returned incomplete metadata for %s/%s/%s", pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version())
	}

	content, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(cacheFile, content, 0644)
	return metadata, err
}

type registryMetadata struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Version   string    `json:"version"`
	Homepage  string    `json:"homepage"`
	Licenses  []string  `json:"licenses"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (m *registryMetadata) IsRecent(migrationThreshold time.Duration) bool {
	return time.Since(m.UpdatedAt) < migrationThreshold
}
