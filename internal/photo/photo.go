package photo

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/smallwat3r/untappd-saver/internal/storage"
)

func DownloadAndSave(ctx context.Context, store storage.Storage, photoURL string, metadata *storage.CheckinMetadata) error {
	if photoURL == "" {
		return nil // Nothing to do
	}

	fmt.Printf("Found photo: %s\n", photoURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photoURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for photo %s: %w", photoURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download photo %s: %w", photoURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download photo %s: status %s", photoURL, resp.Status)
	}

	photoBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read photo bytes: %w", err)
	}

	return store.Upload(ctx, photoBytes, metadata)
}
