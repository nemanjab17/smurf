package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	repoOwner    = "nemanjab17"
	repoName     = "smurf"
	checkInterval = 24 * time.Hour
)

type cachedCheck struct {
	Latest    string    `json:"latest"`
	CheckedAt time.Time `json:"checked_at"`
}

func cacheFile() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "smurf", "update-check.json")
}

// LatestRelease fetches the latest release tag from GitHub.
func LatestRelease() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api: %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

// CheckForUpdate returns the latest version string if a newer version is
// available, or "" if up-to-date / unable to check. Results are cached
// for 24 hours to avoid hitting the API on every invocation.
func CheckForUpdate() string {
	if Version == "dev" {
		return ""
	}

	// Try reading from cache first.
	path := cacheFile()
	if data, err := os.ReadFile(path); err == nil {
		var c cachedCheck
		if json.Unmarshal(data, &c) == nil && time.Since(c.CheckedAt) < checkInterval {
			if c.Latest != Version {
				return c.Latest
			}
			return ""
		}
	}

	latest, err := LatestRelease()
	if err != nil {
		return ""
	}

	// Write cache.
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	c := cachedCheck{Latest: latest, CheckedAt: time.Now()}
	if data, err := json.Marshal(c); err == nil {
		_ = os.WriteFile(path, data, 0644)
	}

	if latest != Version {
		return latest
	}
	return ""
}
