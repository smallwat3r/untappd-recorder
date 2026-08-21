package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decoding must reverse exactly what the uploader's sanitiser applies
func TestDecodeMetadataValue_RoundTrip(t *testing.T) {
	inputs := []string{
		"Brooklyn East IPA",
		"La Bête à Biere",
		"beer 🍺 time",
		"100% great",
		"",
	}
	for _, in := range inputs {
		assert.Equal(t, in, decodeMetadataValue(sanitizeMetadataValue(in)), in)
	}

	// RFC 2047 encoded words from older uploads decode too
	assert.Equal(t, "La Bête", decodeMetadataValue("=?utf-8?q?La_B=C3=AAte?="))
	// a bare literal % that fails percent-decoding is kept as is
	assert.Equal(t, "50% off", decodeMetadataValue("50% off"))
}

func TestMergeManifest(t *testing.T) {
	existing := []ManifestRecord{
		{Key: "2019/01/01/WEBP/1.webp", Beer: "Old", Date: "Tue, 01 Jan 2019 00:00:00 +0000"},
		{Key: "2020/01/01/WEBP/2.webp", Beer: "Kept", Date: "Wed, 01 Jan 2020 00:00:00 +0000"},
	}
	updates := []ManifestRecord{
		// same key: replaces, never duplicates
		{Key: "2019/01/01/WEBP/1.webp", Beer: "New", Date: "Tue, 01 Jan 2019 00:00:00 +0000"},
		{Key: "2021/01/01/WEBP/3.webp", Beer: "Added", Date: "Fri, 01 Jan 2021 00:00:00 +0000"},
	}

	merged := mergeManifest(existing, updates)

	require.Len(t, merged, 3)
	// newest first
	assert.Equal(t, "Added", merged[0].Beer)
	assert.Equal(t, "Kept", merged[1].Beer)
	assert.Equal(t, "New", merged[2].Beer)
}

func manifestMockClient(
	t *testing.T,
	existing []ManifestRecord,
	manifestExists bool,
) (*mockS3Client, *[]byte, **s3.PutObjectInput) {
	t.Helper()
	var putBody []byte
	var putInput *s3.PutObjectInput

	mock := &mockS3Client{
		getObject: func(
			ctx context.Context,
			params *s3.GetObjectInput,
			optFns ...func(*s3.Options),
		) (*s3.GetObjectOutput, error) {
			if !manifestExists {
				return nil, &types.NoSuchKey{}
			}
			b, err := json.Marshal(existing)
			require.NoError(t, err)
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(string(b)))}, nil
		},
		putObject: func(
			ctx context.Context,
			params *s3.PutObjectInput,
			optFns ...func(*s3.Options),
		) (*s3.PutObjectOutput, error) {
			b, err := io.ReadAll(params.Body)
			require.NoError(t, err)
			putBody = b
			putInput = params
			return &s3.PutObjectOutput{}, nil
		},
	}
	return mock, &putBody, &putInput
}

func TestClient_UpdateManifest(t *testing.T) {
	t.Run("no pending uploads is a no-op", func(t *testing.T) {
		mock, putBody, _ := manifestMockClient(t, nil, false)
		client := &Client{s3Client: mock, bucketName: "test-bucket"}

		require.NoError(t, client.UpdateManifest(context.Background()))
		assert.Nil(t, *putBody, "must not touch the bucket with nothing to add")
	})

	t.Run("creates the manifest when missing", func(t *testing.T) {
		mock, putBody, putInput := manifestMockClient(t, nil, false)
		client := &Client{s3Client: mock, bucketName: "test-bucket"}

		md := &CheckinMetadata{
			ID:   "123",
			Beer: "La Bête",
			Date: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
		}
		require.NoError(t, client.UploadWEBP(context.Background(), []byte("webp"), md))
		require.NoError(t, client.UpdateManifest(context.Background()))

		var records []ManifestRecord
		require.NoError(t, json.Unmarshal(*putBody, &records))
		require.Len(t, records, 1)
		assert.Equal(t, "2025/11/01/WEBP/123.webp", records[0].Key)
		assert.Equal(t, "La Bête", records[0].Beer, "manifest must hold decoded UTF-8")
		assert.Equal(t, "Sat, 01 Nov 2025 00:00:00 +0000", records[0].Date)

		require.NotNil(t, *putInput)
		assert.Equal(t, "index.json", *(*putInput).Key)
		assert.Equal(t, "application/json", *(*putInput).ContentType)
		assert.Equal(t, "public, max-age=300", *(*putInput).CacheControl)
	})

	t.Run("failed write keeps records queued for a retry", func(t *testing.T) {
		putAttempts := 0
		var lastBody []byte
		mock := &mockS3Client{
			getObject: func(
				ctx context.Context,
				params *s3.GetObjectInput,
				optFns ...func(*s3.Options),
			) (*s3.GetObjectOutput, error) {
				return nil, &types.NoSuchKey{}
			},
			putObject: func(
				ctx context.Context,
				params *s3.PutObjectInput,
				optFns ...func(*s3.Options),
			) (*s3.PutObjectOutput, error) {
				// the WEBP upload itself must succeed, only the
				// manifest writes are exercised here
				if *params.Key != "index.json" {
					return &s3.PutObjectOutput{}, nil
				}
				putAttempts++
				if putAttempts == 1 {
					return nil, errors.New("transient error")
				}
				b, err := io.ReadAll(params.Body)
				require.NoError(t, err)
				lastBody = b
				return &s3.PutObjectOutput{}, nil
			},
		}
		client := &Client{s3Client: mock, bucketName: "test-bucket"}

		md := &CheckinMetadata{
			ID:   "123",
			Date: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
		}
		require.NoError(t, client.UploadWEBP(context.Background(), []byte("webp"), md))

		require.Error(t, client.UpdateManifest(context.Background()))
		require.NoError(t, client.UpdateManifest(context.Background()))

		var records []ManifestRecord
		require.NoError(t, json.Unmarshal(lastBody, &records))
		assert.Len(t, records, 1, "queued record must survive a failed write")

		// and the queue is drained after the successful write
		require.NoError(t, client.UpdateManifest(context.Background()))
		assert.Equal(t, 2, putAttempts, "a drained queue must not write again")
	})

	t.Run("appends to an existing manifest", func(t *testing.T) {
		existing := []ManifestRecord{{
			Key:  "2020/01/01/WEBP/1.webp",
			Beer: "Older",
			Date: "Wed, 01 Jan 2020 00:00:00 +0000",
		}}
		mock, putBody, _ := manifestMockClient(t, existing, true)
		client := &Client{s3Client: mock, bucketName: "test-bucket"}

		md := &CheckinMetadata{
			ID:   "123",
			Beer: "Newer",
			Date: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
		}
		require.NoError(t, client.UploadWEBP(context.Background(), []byte("webp"), md))
		require.NoError(t, client.UpdateManifest(context.Background()))

		var records []ManifestRecord
		require.NoError(t, json.Unmarshal(*putBody, &records))
		require.Len(t, records, 2)
		assert.Equal(t, "Newer", records[0].Beer, "newest first")
		assert.Equal(t, "Older", records[1].Beer)
	})
}

func TestClient_RebuildManifest(t *testing.T) {
	objects := map[string]map[string]string{
		"2020/01/01/WEBP/1.webp": {
			"id": "1", "beer": "La B%C3%AAte", "date": "Wed, 01 Jan 2020 00:00:00 +0000",
		},
		"2021/01/01/WEBP/2.webp": {
			"id": "2", "beer": "Pale Ale", "date": "Fri, 01 Jan 2021 00:00:00 +0000",
		},
	}
	// JPGs and the manifest itself must be ignored by the walk
	allKeys := []string{
		"2020/01/01/1.jpg", "2020/01/01/WEBP/1.webp",
		"2021/01/01/2.jpg", "2021/01/01/WEBP/2.webp",
		"latest.jpg", "index.json",
	}

	var putBody []byte
	mock := &mockS3Client{
		listObjectsV2: func(
			ctx context.Context,
			params *s3.ListObjectsV2Input,
			optFns ...func(*s3.Options),
		) (*s3.ListObjectsV2Output, error) {
			var contents []types.Object
			for i := range allKeys {
				contents = append(contents, types.Object{Key: &allKeys[i]})
			}
			f := false
			return &s3.ListObjectsV2Output{Contents: contents, IsTruncated: &f}, nil
		},
		headObject: func(
			ctx context.Context,
			params *s3.HeadObjectInput,
			optFns ...func(*s3.Options),
		) (*s3.HeadObjectOutput, error) {
			md, ok := objects[*params.Key]
			if !ok {
				return nil, &types.NotFound{}
			}
			return &s3.HeadObjectOutput{Metadata: md}, nil
		},
		putObject: func(
			ctx context.Context,
			params *s3.PutObjectInput,
			optFns ...func(*s3.Options),
		) (*s3.PutObjectOutput, error) {
			b, err := io.ReadAll(params.Body)
			require.NoError(t, err)
			putBody = b
			return &s3.PutObjectOutput{}, nil
		},
	}

	client := &Client{s3Client: mock, bucketName: "test-bucket"}
	n, err := client.RebuildManifest(context.Background(), 2)

	require.NoError(t, err)
	assert.Equal(t, 2, n)

	var records []ManifestRecord
	require.NoError(t, json.Unmarshal(putBody, &records))
	require.Len(t, records, 2)
	assert.Equal(t, "Pale Ale", records[0].Beer, "newest first")
	assert.Equal(t, "La Bête", records[1].Beer, "metadata must be decoded")
}
