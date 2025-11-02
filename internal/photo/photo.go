package photo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
)

type Downloader interface {
	DownloadAndSave(ctx context.Context, cfg *config.Config, store storage.Storage, photoURL string, metadata *storage.CheckinMetadata) error
}

type DefaultDownloader struct{}

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

func NewDownloader() Downloader {
	return &DefaultDownloader{}
}

func (d *DefaultDownloader) DownloadAndSave(ctx context.Context, cfg *config.Config, store storage.Storage, photoURL string, metadata *storage.CheckinMetadata) error {
	var photoBytes []byte
	var err error

	if photoURL == "" {
		photoBytes, err = usePlaceholderPhoto(cfg.PlaceholderPhotoPath)
		if err != nil {
			return fmt.Errorf("failed to get placeholder photo: %w", err)
		}
	} else {
		photoBytes, err = d.downloadPhoto(ctx, photoURL)
		if err != nil {
			return fmt.Errorf("failed to download photo: %w", err)
		}
	}

	return store.Upload(ctx, photoBytes, metadata)
}

func usePlaceholderPhoto(path string) ([]byte, error) {
	fmt.Printf("No photo found, using default: %s\n", path)
	photoBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read missing photo file %s: %w", path, err)
	}
	return photoBytes, nil
}

func (d *DefaultDownloader) downloadPhoto(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for photo %s: %w", url, err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download photo %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download photo %s: status %s", url, resp.Status)
	}

	return io.ReadAll(resp.Body)
}
