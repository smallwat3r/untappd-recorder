package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/smallwat3r/untappd-saver/internal/config"
	"github.com/smallwat3r/untappd-saver/internal/untappd"
)

type mockS3Client struct {
	putObject     func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	listObjectsV2 func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return m.putObject(ctx, params, optFns...)
}

func (m *mockS3Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return m.listObjectsV2(ctx, params, optFns...)
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
			expectedKey := "12345.jpg"
			if *params.Key != expectedKey {
				t.Errorf("expected key to be '%s', got %s", expectedKey, *params.Key)
			}
			if params.Metadata["latlng"] != "1.230000,4.560000" {
				t.Errorf("expected latlng to be '1.230000,4.560000', got %s", params.Metadata["latlng"])
			}
			if params.Metadata["style"] != "IPA" {
				t.Errorf("expected style to be 'IPA', got %s", params.Metadata["style"])
			}
			if params.Metadata["abv"] != "6.70" {
				t.Errorf("expected abv to be '6.70', got %s", params.Metadata["abv"])
			}
			return &s3.PutObjectOutput{}, nil
		},
	}

	cfg := &config.Config{
		R2BucketName: "test-bucket",
	}

	client := NewR2Client(cfg, mockS3)

	checkin := untappd.Checkin{
		CheckinID: 12345,
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
			BeerName  string  `json:"beer_name"`
			BeerStyle string  `json:"beer_style"`
			BeerABV   float64 `json:"beer_abv"`
		}{
			BeerName:  "test-beer",
			BeerStyle: "IPA",
			BeerABV:   6.7,
		},
		Venue: struct {
			VenueName string `json:"venue_name"`
			Location  struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		}{
			VenueName: "test-venue",
			Location: struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			}{
				Lat: 1.23,
				Lng: 4.56,
			},
		},
	}

	client.SaveCheckin(checkin)

	if !putObjectCalled {
		t.Errorf("expected PutObject to be called, but it wasn't")
	}
}

func TestR2Client_GetLatestCheckinID(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []types.Object{
					{Key: aws.String("123.jpg"), LastModified: aws.Time(time.Now())},
					{Key: aws.String("456.jpg"), LastModified: aws.Time(time.Now().Add(time.Hour))},
				},
			}, nil
		},
	}

	cfg := &config.Config{
		R2BucketName: "test-bucket",
	}

	client := NewR2Client(cfg, mockS3)

	latestID, err := client.GetLatestCheckinID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if latestID != 456 {
		t.Errorf("expected latest ID to be 456, got %d", latestID)
	}
}

func TestR2Client_GetLatestCheckinID_Empty(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []types.Object{},
			}, nil
		},
	}

	cfg := &config.Config{
		R2BucketName: "test-bucket",
	}

	client := NewR2Client(cfg, mockS3)

	latestID, err := client.GetLatestCheckinID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if latestID != 0 {
		t.Errorf("expected latest ID to be 0, got %d", latestID)
	}
}
