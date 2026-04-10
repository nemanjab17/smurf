package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/nemanjab17/smurf/internal/version"
)

func newUpgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade smurf CLI to the latest release",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Current version: %s\n", version.Version)

			latest, err := version.LatestRelease()
			if err != nil {
				return fmt.Errorf("check latest release: %w", err)
			}

			if latest == version.Version {
				fmt.Println("Already up to date.")
				return nil
			}

			fmt.Printf("New version available: %s\n", latest)

			asset := fmt.Sprintf("smurf-%s-%s", runtime.GOOS, runtime.GOARCH)
			url := fmt.Sprintf(
				"https://github.com/nemanjab17/smurf/releases/download/%s/%s",
				latest, asset,
			)

			fmt.Printf("Downloading %s...\n", asset)
			client := &http.Client{Timeout: 60 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				return fmt.Errorf("download: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("download failed: %s", resp.Status)
			}

			// Write to a temp file next to the current binary, then rename.
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("find executable path: %w", err)
			}

			tmp, err := os.CreateTemp("", "smurf-upgrade-*")
			if err != nil {
				return fmt.Errorf("create temp file: %w", err)
			}
			defer os.Remove(tmp.Name())

			if _, err := io.Copy(tmp, resp.Body); err != nil {
				tmp.Close()
				return fmt.Errorf("write binary: %w", err)
			}
			tmp.Close()

			if err := os.Chmod(tmp.Name(), 0755); err != nil {
				return fmt.Errorf("chmod: %w", err)
			}

			if err := os.Rename(tmp.Name(), exe); err != nil {
				return fmt.Errorf("replace binary: %w (try: sudo smurf upgrade)", err)
			}

			fmt.Printf("Upgraded to %s\n", latest)
			return nil
		},
	}
}
