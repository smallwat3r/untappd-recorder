package main

import (
	"context"
	"testing"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
	"github.com/smallwat3r/untappd-recorder/internal/untappd"
)

type mockStorage struct {
	GetLatestCheckinIDFunc    func(ctx context.Context) (uint64, error)
	UpdateLatestCheckinIDFunc func(ctx context.Context, checkin untappd.Checkin) error
	UploadJPGFunc             func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error
	UploadWEBPFunc            func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error
	DownloadFunc              func(ctx context.Context, fileName string) ([]byte, error)
	CheckinExistsFunc         func(ctx context.Context, checkinID, createdAt string) (bool, error)
	CheckinWEBPExistsFunc     func(ctx context.Context, checkinID, createdAt string) (bool, error)
}

func (m *mockStorage) GetLatestCheckinID(ctx context.Context) (uint64, error) {
	if m.GetLatestCheckinIDFunc != nil {
		return m.GetLatestCheckinIDFunc(ctx)
	}
	return 0, nil
}

func (m *mockStorage) UpdateLatestCheckinID(
	ctx context.Context,
	checkin untappd.Checkin,
) error {
	if m.UpdateLatestCheckinIDFunc != nil {
		return m.UpdateLatestCheckinIDFunc(ctx, checkin)
	}
	return nil
}

func (m *mockStorage) UploadJPG(
	ctx context.Context,
	file []byte,
	metadata *storage.CheckinMetadata,
) error {
	if m.UploadJPGFunc != nil {
		return m.UploadJPGFunc(ctx, file, metadata)
	}
	return nil
}

func (m *mockStorage) UploadWEBP(
	ctx context.Context,
	file []byte,
	metadata *storage.CheckinMetadata,
) error {
	if m.UploadWEBPFunc != nil {
		return m.UploadWEBPFunc(ctx, file, metadata)
	}
	return nil
}

func (m *mockStorage) Download(ctx context.Context, fileName string) ([]byte, error) {
	if m.DownloadFunc != nil {
		return m.DownloadFunc(ctx, fileName)
	}
	return nil, nil
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

func (m *mockStorage) CheckinWEBPExists(
	ctx context.Context,
	checkinID, createdAt string,
) (bool, error) {
	if m.CheckinWEBPExistsFunc != nil {
		return m.CheckinWEBPExistsFunc(ctx, checkinID, createdAt)
	}
	return false, nil
}

type mockUntappdClient struct {
	FetchCheckinsFunc func(
		ctx context.Context,
		sinceID uint64,
		checkinProcessor func(context.Context, []untappd.Checkin) error,
	) error
}

func (m *mockUntappdClient) FetchCheckins(
	ctx context.Context,
	sinceID uint64,
	checkinProcessor func(context.Context, []untappd.Checkin) error,
) error {
	if m.FetchCheckinsFunc != nil {
		return m.FetchCheckinsFunc(ctx, sinceID, checkinProcessor)
	}
	return nil
}

type mockDownloader struct {
	DownloadAndSaveFunc func(
		ctx context.Context,
		cfg *config.Config,
		store storage.Storage,
		photoURL string,
		metadata *storage.CheckinMetadata,
	) error
	DownloadAndSaveWEBPFunc func(
		ctx context.Context,
		store storage.Storage,
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

func (m *mockDownloader) DownloadAndSaveWEBP(
	ctx context.Context,
	store storage.Storage,
	metadata *storage.CheckinMetadata,
) error {
	if m.DownloadAndSaveWEBPFunc != nil {
		return m.DownloadAndSaveWEBPFunc(ctx, store, metadata)
	}
	return nil
}

func TestRun_ProcessCheckins(t *testing.T) {
	t.Setenv("UNTAPPD_ACCESS_TOKEN", "test-token")
	t.Setenv("R2_ACCOUNT_ID", "test-account-id")
	t.Setenv("R2_ACCESS_KEY_ID", "test-key-id")
	t.Setenv("R2_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("BUCKET_NAME", "test-bucket")
	t.Setenv("NUM_WORKERS", "1")

	updateLatestCheckinIDCalled := false

	mockStore := &mockStorage{
		UpdateLatestCheckinIDFunc: func(
			ctx context.Context,
			checkin untappd.Checkin,
		) error {
			updateLatestCheckinIDCalled = true

			if checkin.CheckinID != 54321 {
				t.Errorf("expected checkinID to be 54321, got %d", checkin.CheckinID)
			}

			return nil
		},
	}

	downloadAndSaveCalled := false

	mockDownloader := &mockDownloader{
		DownloadAndSaveFunc: func(
			ctx context.Context,
			cfg *config.Config,
			store storage.Storage,
			photoURL string,
			metadata *storage.CheckinMetadata,
		) error {
			downloadAndSaveCalled = true

			if metadata.ID != "54321" {
				t.Errorf("expected metadata ID to be 54321, got %s", metadata.ID)
			}

			return nil
		},
	}

	mockUntappd := &mockUntappdClient{
		FetchCheckinsFunc: func(
			ctx context.Context,
			sinceID uint64,
			checkinProcessor func(context.Context, []untappd.Checkin) error,
		) error {
			checkins := []untappd.Checkin{
				{CheckinID: 54321},
			}
			return checkinProcessor(ctx, checkins)
		},
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if err := runRecorder(
		context.Background(),
		mockStore,
		cfg,
		mockUntappd,
		mockDownloader,
	); err != nil {
		t.Errorf("runRecorder() error = %v, wantErr %v", err, false)
	}

	if !updateLatestCheckinIDCalled {
		t.Error("expected UpdateLatestCheckinID to be called, but it was not")
	}

	if !downloadAndSaveCalled {
		t.Error("expected DownloadAndSave to be called, but it was not")
	}
}

func TestRun(t *testing.T) {
	t.Setenv("UNTAPPD_ACCESS_TOKEN", "test-token")
	t.Setenv("R2_ACCOUNT_ID", "test-account-id")
	t.Setenv("R2_ACCESS_KEY_ID", "test-key-id")
	t.Setenv("R2_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("BUCKET_NAME", "test-bucket")
	t.Setenv("NUM_WORKERS", "1")

	getLatestCheckinIDCalled := false

	mockStore := &mockStorage{
		GetLatestCheckinIDFunc: func(ctx context.Context) (uint64, error) {
			getLatestCheckinIDCalled = true
			return 123, nil
		},
	}

	fetchCheckinsCalled := false

	mockUntappd := &mockUntappdClient{
		FetchCheckinsFunc: func(
			ctx context.Context,
			sinceID uint64,
			checkinProcessor func(context.Context, []untappd.Checkin) error,
		) error {
			fetchCheckinsCalled = true

			if sinceID != 123 {
				t.Errorf("expected sinceID to be 123, got %d", sinceID)
			}

			return nil
		},
	}

	if err := run(
		context.Background(),
		mockStore,
		mockUntappd,
	); err != nil {
		t.Errorf("run() error = %v, wantErr %v", err, false)
	}

	if !getLatestCheckinIDCalled {
		t.Error("expected GetLatestCheckinID to be called, but it was not")
	}

	if !fetchCheckinsCalled {
		t.Error("expected FetchCheckins to be called, but it was not")
	}
}
