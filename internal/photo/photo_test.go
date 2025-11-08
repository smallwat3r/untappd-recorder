package photo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
	"github.com/smallwat3r/untappd-recorder/internal/untappd"
)

func TestMain(m *testing.M) {
	vips.Startup(nil)
	defer vips.Shutdown()
	os.Exit(m.Run())
}

type mockStorage struct {
	UploadJPGFunc             func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error
	UploadAVIFFunc            func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error
	DownloadFunc              func(ctx context.Context, fileName string) ([]byte, error)
	CheckinExistsFunc         func(ctx context.Context, checkinID, createdAt string) (bool, error)
	CheckinAVIFExistsFunc     func(ctx context.Context, checkinID, createdAt string) (bool, error)
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

func (m *mockStorage) UploadAVIF(
	ctx context.Context,
	file []byte,
	metadata *storage.CheckinMetadata,
) error {
	if m.UploadAVIFFunc != nil {
		return m.UploadAVIFFunc(ctx, file, metadata)
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

func (m *mockStorage) CheckinAVIFExists(
	ctx context.Context,
	checkinID, createdAt string,
) (bool, error) {
	if m.CheckinAVIFExistsFunc != nil {
		return m.CheckinAVIFExistsFunc(ctx, checkinID, createdAt)
	}
	return false, nil
}

func (m *mockStorage) GetLatestCheckinID(ctx context.Context) (uint64, error) {
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

func TestDefaultDownloader_DownloadAndSave(t *testing.T) {
	// read a real image file for testing govips
	imgData, err := os.ReadFile("../../img/missing.jpg")
	if err != nil {
		t.Fatalf("failed to read missing.jpg: %v", err)
	}

	t.Setenv("PLACEHOLDER_PHOTO_PATH", "../../img/missing.jpg")
	cfg, _ := config.Load()

	tests := []struct {
		name                    string
		photoURL                string
		serverStatus            int
		serverBody              []byte
		expectedUploadJPGCalls  int
		expectedUploadAVIFCalls int
		expectedErr             bool
	}{
		{
			name:                    "Successful download and save",
			photoURL:                "http://example.com/photo.jpg",
			serverStatus:            http.StatusOK,
			serverBody:              imgData,
			expectedUploadJPGCalls:  1,
			expectedUploadAVIFCalls: 1,
			expectedErr:             false,
		},
		{
			name:                    "Placeholder photo used",
			photoURL:                "",
			serverStatus:            http.StatusOK, // not used for placeholder
			serverBody:              nil,
			expectedUploadJPGCalls:  1,
			expectedUploadAVIFCalls: 1,
			expectedErr:             false,
		},
		{
			name:                    "Download failed - non-200 status",
			photoURL:                "http://example.com/photo.jpg",
			serverStatus:            http.StatusNotFound,
			serverBody:              []byte("not found"),
			expectedUploadJPGCalls:  0,
			expectedUploadAVIFCalls: 0,
			expectedErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				w.Write(tt.serverBody)
			}))
			defer server.Close()

			downloader := NewDownloader()
			metadata := &storage.CheckinMetadata{ID: "123", Date: "Sat, 01 Nov 2025 00:00:00 +0000"}

			var uploadJPGCalls int
			var uploadAVIFCalls int
			mockStore := &mockStorage{
				UploadJPGFunc: func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error {
					uploadJPGCalls++
					return nil
				},
				UploadAVIFFunc: func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error {
					uploadAVIFCalls++
					return nil
				},
				DownloadFunc: func(ctx context.Context, fileName string) ([]byte, error) {
					// mock download for AVIF conversion from JPG
					return imgData, nil
				},
			}

			photoURL := tt.photoURL
			if photoURL != "" {
				photoURL = server.URL + "/photo.jpg"
			}

			err := downloader.DownloadAndSave(context.Background(), cfg, mockStore, photoURL, metadata)

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
				t.Errorf("expected %d UploadJPG calls, got %d", tt.expectedUploadJPGCalls, uploadJPGCalls)
			}
			if uploadAVIFCalls != tt.expectedUploadAVIFCalls {
				t.Errorf("expected %d UploadAVIF calls, got %d", tt.expectedUploadAVIFCalls, uploadAVIFCalls)
			}
		})
	}
}

func TestDefaultDownloader_DownloadAndSaveAVIF(t *testing.T) {
	// read a real image file for testing govips
	imgData, err := os.ReadFile("../../img/missing.jpg")
	if err != nil {
		t.Fatalf("failed to read missing.jpg: %v", err)
	}

	tests := []struct {
		name                    string
		expectedDownloadCalls   int
		expectedUploadAVIFCalls int
		expectedErr             bool
	}{
		{
			name:                    "Successful AVIF conversion from downloaded JPG",
			expectedDownloadCalls:   1,
			expectedUploadAVIFCalls: 1,
			expectedErr:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			downloader := NewDownloader()
			metadata := &storage.CheckinMetadata{ID: "123", Date: "Sat, 01 Nov 2025 00:00:00 +0000"}

			var downloadCalls int
			var uploadAVIFCalls int
			mockStore := &mockStorage{
				DownloadFunc: func(ctx context.Context, fileName string) ([]byte, error) {
					downloadCalls++
					return imgData, nil
				},
				UploadAVIFFunc: func(ctx context.Context, file []byte, metadata *storage.CheckinMetadata) error {
					uploadAVIFCalls++
					return nil
				},
			}

			err := downloader.DownloadAndSaveAVIF(context.Background(), mockStore, metadata)

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
				t.Errorf("expected %d Download calls, got %d", tt.expectedDownloadCalls, downloadCalls)
			}
			if uploadAVIFCalls != tt.expectedUploadAVIFCalls {
				t.Errorf("expected %d UploadAVIF calls, got %d", tt.expectedUploadAVIFCalls, uploadAVIFCalls)
			}
		})
	}
}
