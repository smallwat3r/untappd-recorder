package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/photo"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
	"github.com/smallwat3r/untappd-recorder/internal/untappd"
)

type mockStorage struct {
	CheckinExistsFunc         func(ctx context.Context, checkinID, createdAt string) (bool, error)
	UploadFunc                func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error
	DownloadFunc              func(ctx context.Context, fileName string) ([]byte, error)
	GetLatestCheckinIDFunc    func(ctx context.Context) (int, error)
	UpdateLatestCheckinIDFunc func(ctx context.Context, checkin untappd.Checkin) error
}

func (m *mockStorage) CheckinExists(
	ctx context.Context,
	checkinID, createdAt string,
) (bool, error) {
	if m.CheckinExistsFunc != nil {
		return m.CheckinExistsFunc(ctx, checkinID, createdAt)
	}
	return false, nil
}

func (m *mockStorage) GetLatestCheckinID(ctx context.Context) (int, error) {
	if m.GetLatestCheckinIDFunc != nil {
		return m.GetLatestCheckinIDFunc(ctx)
	}
	return 0, nil
}

func (m *mockStorage) UpdateLatestCheckinID(ctx context.Context, checkin untappd.Checkin) error {
	if m.UpdateLatestCheckinIDFunc != nil {
		return m.UpdateLatestCheckinIDFunc(ctx, checkin)
	}
	return nil
}

func (m *mockStorage) Upload(
	ctx context.Context,
	file []byte,
	metadata *storage.CheckinMetadata,
) error {
	if m.UploadFunc != nil {
		return m.UploadFunc(ctx, file, metadata)
	}
	return nil
}

func (m *mockStorage) Download(ctx context.Context, fileName string) ([]byte, error) {
	if m.DownloadFunc != nil {
		return m.DownloadFunc(ctx, fileName)
	}
	return nil, nil
}

type mockDownloader struct {
	DownloadAndSaveFunc func(
		ctx context.Context,
		cfg *config.Config,
		store storage.Storage,
		photoURL string,
		metadata *storage.CheckinMetadata,
	) error
}

func (m *mockDownloader) DownloadAndSave(
	ctx context.Context,
	cfg *config.Config,
	store storage.Storage,
	photoURL string,
	metadata *storage.CheckinMetadata,
) error {
	if m.DownloadAndSaveFunc != nil {
		return m.DownloadAndSaveFunc(ctx, cfg, store, photoURL, metadata)
	}
	return nil
}

func TestRun(t *testing.T) {
	tempDir := t.TempDir()

	t.Setenv("UNTAPPD_ACCESS_TOKEN", "test-token")
	t.Setenv("R2_ACCOUNT_ID", "test-account-id")
	t.Setenv("R2_ACCESS_KEY_ID", "test-key-id")
	t.Setenv("R2_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("BUCKET_NAME", "test-bucket")
	t.Setenv("NUM_WORKERS", "1")

	// dummy CSV file content
	csvContent := `checkin_id,created_at,photo_url,beer_name,brewery_name,beer_type,beer_abv,beer_ibu,comment,venue_name,venue_city,venue_state,venue_country,venue_lat,venue_lng,rating_score,checkin_url,beer_url,brewery_url,brewery_country,brewery_city,brewery_state,flavor_profiles,purchase_venue,bid,brewery_id,global_rating_score,global_weighted_rating_score,tagged_friends,total_toasts,total_comments
12345,2023-01-01 12:00:00,http://example.com/photo.jpg,Test Beer,Test Brewery,IPA,5.0,50,Test comment,Test Venue,Test City,Test State,Test Country,1.23,4.56,4.5,http://example.com/checkin,http://example.com/beer,http://example.com/brewery,Brewery Country,Brewery City,Brewery State,Hoppy,Test Store,67890,123,4.0,4.2,"",0,0
`
	csvPath := filepath.Join(tempDir, "test.csv")
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to create test CSV file: %v", err)
	}

	checkinExistsCalled := false

	mockStore := &mockStorage{
		CheckinExistsFunc: func(
			ctx context.Context,
			checkinID, createdAt string,
		) (bool, error) {
			checkinExistsCalled = true

			if checkinID != "12345" {
				t.Errorf("expected checkinID to be 12345, got %s", checkinID)
			}
			if createdAt != "2023-01-01 12:00:00" {
				t.Errorf("expected createdAt to be 2023-01-01 12:00:00, got %s", createdAt)
			}

			return false, nil
		},
	}

	downloadAndSaveCalled := false

	var downloader photo.Downloader = &mockDownloader{
		DownloadAndSaveFunc: func(
			ctx context.Context,
			cfg *config.Config,
			store storage.Storage,
			photoURL string,
			metadata *storage.CheckinMetadata,
		) error {
			downloadAndSaveCalled = true

			if photoURL != "http://example.com/photo.jpg" {
				t.Errorf("expected photoURL to be http://example.com/photo.jpg, got %s", photoURL)
			}

			return nil
		},
	}

	if err := run(context.Background(), csvPath, mockStore, downloader); err != nil {
		t.Errorf("run() error = %v, wantErr %v", err, false)
	}

	if !checkinExistsCalled {
		t.Error("Expected CheckinExists to be called, but it was not")
	}
	if !downloadAndSaveCalled {
		t.Error("Expected DownloadAndSave to be called, but it was not")
	}
}
