package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/stretchr/testify/assert"
)

type mockS3Client struct {
	putObject func(
		ctx context.Context,
		params *s3.PutObjectInput,
		optFns ...func(*s3.Options),
	) (*s3.PutObjectOutput, error)

	getObject func(
		ctx context.Context,
		params *s3.GetObjectInput,
		optFns ...func(*s3.Options),
	) (*s3.GetObjectOutput, error)

	copyObject func(
		ctx context.Context,
		params *s3.CopyObjectInput,
		optFns ...func(*s3.Options),
	) (*s3.CopyObjectOutput, error)

	headObject func(
		ctx context.Context,
		params *s3.HeadObjectInput,
		optFns ...func(*s3.Options),
	) (*s3.HeadObjectOutput, error)

	listObjectsV2 func(
		ctx context.Context,
		params *s3.ListObjectsV2Input,
		optFns ...func(*s3.Options),
	) (*s3.ListObjectsV2Output, error)
}

func (m *mockS3Client) ListObjectsV2(
	ctx context.Context,
	params *s3.ListObjectsV2Input,
	optFns ...func(*s3.Options),
) (*s3.ListObjectsV2Output, error) {
	return m.listObjectsV2(ctx, params, optFns...)
}

func (m *mockS3Client) PutObject(
	ctx context.Context,
	params *s3.PutObjectInput,
	optFns ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	return m.putObject(ctx, params, optFns...)
}

func (m *mockS3Client) GetObject(
	ctx context.Context,
	params *s3.GetObjectInput,
	optFns ...func(*s3.Options),
) (*s3.GetObjectOutput, error) {
	return m.getObject(ctx, params, optFns...)
}

func (m *mockS3Client) CopyObject(
	ctx context.Context,
	params *s3.CopyObjectInput,
	optFns ...func(*s3.Options),
) (*s3.CopyObjectOutput, error) {
	return m.copyObject(ctx, params, optFns...)
}

func (m *mockS3Client) HeadObject(
	ctx context.Context,
	params *s3.HeadObjectInput,
	optFns ...func(*s3.Options),
) (*s3.HeadObjectOutput, error) {
	if m.headObject == nil {
		return nil, nil
	}
	return m.headObject(ctx, params, optFns...)
}

func TestClient_UploadJPG(t *testing.T) {
	var putObjectCalled bool
	mockS3 := &mockS3Client{
		putObject: func(
			ctx context.Context,
			params *s3.PutObjectInput,
			optFns ...func(*s3.Options),
		) (*s3.PutObjectOutput, error) {
			putObjectCalled = true
			if *params.Bucket != "test-bucket" {
				t.Errorf("expected bucket to be 'test-bucket', got %s", *params.Bucket)
			}
			expectedKey := "2025/11/01/123.jpg"
			if *params.Key != expectedKey {
				t.Errorf("expected key to be '%s', got %s", expectedKey, *params.Key)
			}
			return &s3.PutObjectOutput{}, nil
		},
	}

	client := &Client{
		s3Client:   mockS3,
		bucketName: "test-bucket",
	}

	metadata := &CheckinMetadata{
		ID:   "123",
		Date: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
	}
	err := client.UploadJPG(context.Background(), []byte("test"), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !putObjectCalled {
		t.Errorf("expected PutObject to be called, but it wasn't")
	}
}

func TestClient_UploadWEBP(t *testing.T) {
	mockClient := &mockS3Client{
		putObject: func(
			ctx context.Context,
			params *s3.PutObjectInput,
			optFns ...func(*s3.Options),
		) (*s3.PutObjectOutput, error) {
			return &s3.PutObjectOutput{}, nil
		},
	}

	client := &Client{s3Client: mockClient, bucketName: "test-bucket"}
	metadata := &CheckinMetadata{
		ID:   "123",
		Date: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
	}

	err := client.UploadWEBP(context.Background(), []byte("webp-data"), metadata)
	assert.NoError(t, err)
}

func TestClient_CheckinWEBPExists(t *testing.T) {
	t.Run("WEBP_exists", func(t *testing.T) {
		mockClient := &mockS3Client{
			headObject: func(
				ctx context.Context,
				params *s3.HeadObjectInput,
				optFns ...func(*s3.Options),
			) (*s3.HeadObjectOutput, error) {
				return &s3.HeadObjectOutput{}, nil
			},
		}

		client := &Client{s3Client: mockClient, bucketName: "test-bucket"}
		exists, err := client.CheckinWEBPExists(context.Background(), "123", "2025-11-01 00:00:00")

		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("WEBP_does_not_exist", func(t *testing.T) {
		mockClient := &mockS3Client{
			headObject: func(
				ctx context.Context,
				params *s3.HeadObjectInput,
				optFns ...func(*s3.Options),
			) (*s3.HeadObjectOutput, error) {
				return nil, &types.NotFound{}
			},
		}

		client := &Client{s3Client: mockClient, bucketName: "test-bucket"}
		exists, err := client.CheckinWEBPExists(context.Background(), "123", "2025-11-01 00:00:00")

		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("Error_on_HeadObject", func(t *testing.T) {
		mockClient := &mockS3Client{
			headObject: func(
				ctx context.Context,
				params *s3.HeadObjectInput,
				optFns ...func(*s3.Options),
			) (*s3.HeadObjectOutput, error) {
				return nil, errors.New("some error")
			},
		}

		client := &Client{s3Client: mockClient, bucketName: "test-bucket"}
		_, err := client.CheckinWEBPExists(context.Background(), "123", "2025-11-01 00:00:00")

		assert.Error(t, err)
	})
}

func TestClient_ListJPGKeys(t *testing.T) {
	// two pages; only .jpg keys must be returned
	pages := [][]string{
		{"2019/08/18/1.jpg", "2019/08/18/WEBP/1.webp", "latest.jpg"},
		{"2019/08/19/2.jpg"},
	}

	var call int
	mockClient := &mockS3Client{
		listObjectsV2: func(
			ctx context.Context,
			params *s3.ListObjectsV2Input,
			optFns ...func(*s3.Options),
		) (*s3.ListObjectsV2Output, error) {
			page := pages[call]
			call++

			var contents []types.Object
			for _, k := range page {
				contents = append(contents, types.Object{Key: aws.String(k)})
			}
			truncated := call < len(pages)
			return &s3.ListObjectsV2Output{
				Contents:              contents,
				IsTruncated:           aws.Bool(truncated),
				NextContinuationToken: aws.String("next"),
			}, nil
		},
	}

	client := &Client{s3Client: mockClient, bucketName: "test-bucket"}
	keys, err := client.ListJPGKeys(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, []string{"2019/08/18/1.jpg", "latest.jpg", "2019/08/19/2.jpg"}, keys)
}

func TestWEBPSiblingKey(t *testing.T) {
	assert.Equal(t, "2019/08/18/WEBP/111.webp", WEBPSiblingKey("2019/08/18/111.jpg"))
	assert.Equal(t, "", WEBPSiblingKey("latest.jpg"))

	// must stay consistent with CheckinKey's layout
	d := time.Date(2019, 8, 18, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, CheckinKey(d, "111", "webp"), WEBPSiblingKey(CheckinKey(d, "111", "jpg")))
}

func TestNewClient_R2(t *testing.T) {
	cfg := &config.Config{
		R2AccountID:       "test-account-id",
		R2AccessKeyID:     "test-key-id",
		R2AccessKeySecret: "test-key-secret",
		BucketName:        "test-bucket",
	}

	_, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewClient_S3(t *testing.T) {
	cfg := &config.Config{
		BucketName: "test-bucket",
		AWSRegion:  "us-east-1",
	}

	_, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
