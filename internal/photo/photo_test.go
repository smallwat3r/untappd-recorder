package photo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
	"github.com/smallwat3r/untappd-recorder/internal/untappd"
	"github.com/smallwat3r/untappd-recorder/internal/vips"
)

func TestMain(m *testing.M) {
	vips.Startup(nil)
	defer vips.Shutdown()
	os.Exit(m.Run())
}

type mockStorage struct {
	UploadJPGFunc             func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error
	UploadWEBPFunc            func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error
	DownloadFunc              func(ctx context.Context, fileName string) ([]byte, error)
	CheckinExistsFunc         func(ctx context.Context, checkinID, createdAt string) (bool, error)
	CheckinWEBPExistsFunc     func(ctx context.Context, checkinID, createdAt string) (bool, error)
	GetLatestCheckinIDFunc    func(ctx context.Context) (uint64, error)
	UpdateLatestCheckinIDFunc func(ctx context.Context, checkin untappd.Checkin) error
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

func (m *mockStorage) Download(
	ctx context.Context,
	fileName string,
) ([]byte, error) {
	if m.DownloadFunc != nil {
		return m.DownloadFunc(ctx, fileName)
	}
	return nil, nil
}

func (m *mockStorage) CheckinExists(
	ctx context.Context,
	checkinID,
	createdAt string,
) (bool, error) {
	if m.CheckinExistsFunc != nil {
		return m.CheckinExistsFunc(ctx, checkinID, createdAt)
	}
	return false, nil
}

func (m *mockStorage) CheckinWEBPExists(
	ctx context.Context,
	checkinID,
	createdAt string,
) (bool, error) {
	if m.CheckinWEBPExistsFunc != nil {
		return m.CheckinWEBPExistsFunc(ctx, checkinID, createdAt)
	}
	return false, nil
}

func (m *mockStorage) GetLatestCheckinID(
	ctx context.Context,
) (uint64, error) {
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

func TestDefaultDownloader_DownloadAndSave(t *testing.T) {
	// Read a real image file for testing govips
	imgData, err := os.ReadFile("../../img/missing.jpg")
	if err != nil {
		t.Fatalf("failed to read missing.jpg: %v", err)
	}

	tests := []struct {
		name                    string
		photoURL                string
		serverStatus            int
		serverBody              []byte
		expectedUploadJPGCalls  int
		expectedUploadWEBPCalls int
		expectedErr             bool
	}{
		{
			name:                    "Successful download and save",
			photoURL:                "http://example.com/photo.jpg",
			serverStatus:            http.StatusOK,
			serverBody:              imgData,
			expectedUploadJPGCalls:  1,
			expectedUploadWEBPCalls: 1,
			expectedErr:             false,
		},
		{
			name:                    "Placeholder photo used",
			photoURL:                "",
			serverStatus:            http.StatusOK, // not used for placeholder
			serverBody:              nil,
			expectedUploadJPGCalls:  1,
			expectedUploadWEBPCalls: 1,
			expectedErr:             false,
		},
		{
			name:                    "Download failed - non-200 status",
			photoURL:                "http://example.com/photo.jpg",
			serverStatus:            http.StatusNotFound,
			serverBody:              []byte("not found"),
			expectedUploadJPGCalls:  0,
			expectedUploadWEBPCalls: 0,
			expectedErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLACEHOLDER_PHOTO_PATH", "../../img/missing.jpg")
			t.Setenv("UNTAPPD_ACCESS_TOKEN", "test")
			t.Setenv("BUCKET_NAME", "test")
			t.Setenv("NUM_WORKERS", "1")
			t.Setenv("UNTAPPD_ACCESS_TOKEN", "test_token")
			t.Setenv("R2_ACCOUNT_ID", "test_account_id")
			t.Setenv("R2_ACCESS_KEY_ID", "test_key_id")
			t.Setenv("R2_SECRET_ACCESS_KEY", "test_key_secret")
			t.Setenv("BUCKET_NAME", "test_bucket_name")
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("failed to load config: %v", err)
			}

			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.serverStatus)
					w.Write(tt.serverBody)
				}),
			)
			defer server.Close()

			downloader := NewDownloader()
			metadata := &storage.CheckinMetadata{
				ID:   "123",
				Date: "Sat, 01 Nov 2025 00:00:00 +0000",
			}

			var uploadJPGCalls int
			var uploadWEBPCalls int

			mockStore := &mockStorage{
				UploadJPGFunc: func(
					ctx context.Context,
					file []byte,
					metadata *storage.CheckinMetadata,
				) error {
					uploadJPGCalls++
					return nil
				},
				UploadWEBPFunc: func(
					ctx context.Context,
					file []byte,
					metadata *storage.CheckinMetadata,
				) error {
					uploadWEBPCalls++
					return nil
				},
				DownloadFunc: func(
					ctx context.Context,
					fileName string,
				) ([]byte, error) {
					// mock download for WEBP conversion from JPG
					return imgData, nil
				},
			}

			photoURL := tt.photoURL
			if photoURL != "" {
				photoURL = server.URL + "/photo.jpg"
			}

			err = downloader.DownloadAndSave(
				context.Background(),
				cfg,
				mockStore,
				photoURL,
				metadata,
			)

			if tt.expectedErr {
				if err == nil {
					t.Errorf("expected an error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if uploadJPGCalls != tt.expectedUploadJPGCalls {
				t.Errorf(
					"expected %d UploadJPG calls, got %d",
					tt.expectedUploadJPGCalls,
					uploadJPGCalls,
				)
			}

			if uploadWEBPCalls != tt.expectedUploadWEBPCalls {
				t.Errorf(
					"expected %d UploadWEBP calls, got %d",
					tt.expectedUploadWEBPCalls,
					uploadWEBPCalls,
				)
			}
		})
	}
}

func TestDefaultDownloader_DownloadAndSaveWEBP(t *testing.T) {
	// read a real image file for testing govips
	imgData, err := os.ReadFile("../../img/missing.jpg")
	if err != nil {
		t.Fatalf("failed to read missing.jpg: %v", err)
	}

	tests := []struct {
		name                    string
		expectedDownloadCalls   int
		expectedUploadWEBPCalls int
		expectedErr             bool
	}{
		{
			name:                    "Successful WEBP conversion from downloaded JPG",
			expectedDownloadCalls:   1,
			expectedUploadWEBPCalls: 1,
			expectedErr:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLACEHOLDER_PHOTO_PATH", "../../img/missing.jpg")
			t.Setenv("UNTAPPD_ACCESS_TOKEN", "test")
			t.Setenv("BUCKET_NAME", "test")
			t.Setenv("NUM_WORKERS", "1")
			t.Setenv("UNTAPPD_ACCESS_TOKEN", "test_token")
			t.Setenv("R2_ACCOUNT_ID", "test_account_id")
			t.Setenv("R2_ACCESS_KEY_ID", "test_key_id")
			t.Setenv("R2_SECRET_ACCESS_KEY", "test_key_secret")
			t.Setenv("BUCKET_NAME", "test_bucket_name")
			_, err := config.Load()
			if err != nil {
				t.Fatalf("failed to load config: %v", err)
			}

			downloader := NewDownloader()
			metadata := &storage.CheckinMetadata{
				ID:   "123",
				Date: "Sat, 01 Nov 2025 00:00:00 +0000",
			}

			var downloadCalls int
			var uploadWEBPCalls int

			mockStore := &mockStorage{
				DownloadFunc: func(
					ctx context.Context,
					fileName string,
				) ([]byte, error) {
					downloadCalls++
					return imgData, nil
				},
				UploadWEBPFunc: func(
					ctx context.Context,
					file []byte,
					metadata *storage.CheckinMetadata,
				) error {
					uploadWEBPCalls++
					return nil
				},
			}

			err = downloader.DownloadAndSaveWEBP(
				context.Background(),
				mockStore,
				metadata,
			)

			if tt.expectedErr {
				if err == nil {
					t.Errorf("expected an error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if downloadCalls != tt.expectedDownloadCalls {
				t.Errorf(
					"expected %d Download calls, got %d",
					tt.expectedDownloadCalls,
					downloadCalls,
				)
			}

			if uploadWEBPCalls != tt.expectedUploadWEBPCalls {
				t.Errorf(
					"expected %d UploadWEBP calls, got %d",
					tt.expectedUploadWEBPCalls,
					uploadWEBPCalls,
				)
			}
		})
	}
}
