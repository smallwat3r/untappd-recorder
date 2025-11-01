package storage

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/smallwat3r/untappd-recorder/internal/config"
)

type mockS3Client struct {
	putObject     func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	getObject     func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	copyObject    func(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	headObject    func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	listObjectsV2 func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return m.putObject(ctx, params, optFns...)
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return m.getObject(ctx, params, optFns...)
}

func (m *mockS3Client) CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	return m.copyObject(ctx, params, optFns...)
}

func (m *mockS3Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return m.listObjectsV2(ctx, params, optFns...)
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headObject == nil {
		return nil, nil
	}
	return m.headObject(ctx, params, optFns...)
}

func TestClient_Upload(t *testing.T) {
	var putObjectCalled bool
	mockS3 := &mockS3Client{
		putObject: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
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
		Date: "Sat, 01 Nov 2025 00:00:00 +0000",
	}
	err := client.Upload(context.Background(), []byte("test"), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !putObjectCalled {
		t.Errorf("expected PutObject to be called, but it wasn't")
	}
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
