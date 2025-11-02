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
	DownloadAndSave(
		ctx context.Context,
		cfg *config.Config,
		store storage.Storage,
		photoURL string,
		metadata *storage.CheckinMetadata,
	) error
}

type DefaultDownloader struct{}

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

func NewDownloader() Downloader {
	return &DefaultDownloader{}
}

func (d *DefaultDownloader) DownloadAndSave(
	ctx context.Context,
	cfg *config.Config,
	store storage.Storage,
	photoURL string,
	metadata *storage.CheckinMetadata,
) error {
	var (
		b   []byte
		err error
	)

	if photoURL == "" {
		b, err = usePlaceholderPhoto(cfg.PlaceholderPhotoPath)
	} else {
		b, err = d.downloadPhoto(ctx, photoURL)
	}
	if err != nil {
		return fmt.Errorf("failed to get photo: %w", err)
	}

	if err := store.Upload(ctx, b, metadata); err != nil {
		return fmt.Errorf("failed to upload photo: %w", err)
	}
	return nil
}

func usePlaceholderPhoto(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read placeholder file %q: %w", path, err)
	}
	return b, nil
}

const maxPhotoBytes = 10 << 20 // 10 MiB

func (d *DefaultDownloader) downloadPhoto(ctx context.Context, urlStr string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed request for photo %q: %w", urlStr, err)
	}
	req.Header.Set("User-Agent", "untappd-recorder/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download photo %q: %w", urlStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// drain a small amount so connection can be reused
		io.CopyN(io.Discard, resp.Body, 512)
		return nil, fmt.Errorf("failed to download photo %q: status %s", urlStr, resp.Status)
	}

	// cap the read to prevent huge responses
	limited := io.LimitReader(resp.Body, maxPhotoBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read photo %q: %w", urlStr, err)
	}
	if int64(len(data)) > maxPhotoBytes {
		return nil, fmt.Errorf("failed to download photo %q: exceeds %d bytes", urlStr, maxPhotoBytes)
	}

	return data, nil
}
