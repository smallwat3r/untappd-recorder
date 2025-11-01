package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/smallwat3r/untappd-saver/internal/config"
	"github.com/smallwat3r/untappd-saver/internal/untappd"
)

type mockS3Client struct {
	putObject func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return m.putObject(ctx, params, optFns...)
}

func TestR2Client_SaveCheckin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test-image"))
	}))
	defer server.Close()

	var putObjectCalled bool
	mockS3 := &mockS3Client{
		putObject: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			putObjectCalled = true
			if *params.Bucket != "test-bucket" {
				t.Errorf("expected bucket to be 'test-bucket', got %s", *params.Bucket)
			}
			expectedKey := "test-brewery-test-beer.jpg"
			if *params.Key != expectedKey {
				t.Errorf("expected key to be '%s', got %s", expectedKey, *params.Key)
			}
			return &s3.PutObjectOutput{}, nil
		},
	}

	cfg := &config.Config{
		R2BucketName: "test-bucket",
	}

	client := NewR2Client(cfg, mockS3)

	checkin := untappd.Checkin{
		Media: struct {
			Items []struct {
				Photo struct {
					PhotoImgOg string `json:"photo_img_og"`
				} `json:"photo"`
			} `json:"items"`
		}{
			Items: []struct {
				Photo struct {
					PhotoImgOg string `json:"photo_img_og"`
				} `json:"photo"`
			}{
				{
					Photo: struct {
						PhotoImgOg string `json:"photo_img_og"`
					}{
						PhotoImgOg: server.URL,
					},
				},
			},
		},
		Brewery: struct {
			BreweryName string `json:"brewery_name"`
		}{
			BreweryName: "test-brewery",
		},
		Beer: struct {
			BeerName string `json:"beer_name"`
		}{
			BeerName: "test-beer",
		},
	}

	client.SaveCheckin(checkin)

	if !putObjectCalled {
		t.Errorf("expected PutObject to be called, but it wasn't")
	}
}
