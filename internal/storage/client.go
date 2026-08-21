package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/smallwat3r/untappd-recorder/internal/config"
)

type Client struct {
	s3Client   S3Client
	bucketName string
}

func NewClient(ctx context.Context, cfg *config.Config) (*Client, error) {
	switch {
	case cfg.R2AccountID != "":
		return newR2Client(ctx, cfg)
	case cfg.AWSRegion != "":
		return newS3Client(ctx, cfg)
	default:
		return nil, fmt.Errorf("no storage provider configured")
	}
}

func newR2Client(ctx context.Context, cfg *config.Config) (*Client, error) {
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID)

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.R2AccessKeyID,
				cfg.R2AccessKeySecret,
				"",
			),
		),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for R2: %w", err)
	}

	s3c := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return &Client{
		s3Client:   s3c,
		bucketName: cfg.BucketName,
	}, nil
}

func newS3Client(ctx context.Context, cfg *config.Config) (*Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for S3: %w", err)
	}

	return &Client{
		s3Client:   s3.NewFromConfig(awsCfg),
		bucketName: cfg.BucketName,
	}, nil
}

func (c *Client) UploadJPG(ctx context.Context, file []byte, md *CheckinMetadata) error {
	return c.upload(ctx, file, md, "jpg", "image/jpeg")
}

func (c *Client) UploadWEBP(ctx context.Context, file []byte, md *CheckinMetadata) error {
	return c.upload(ctx, file, md, "webp", "image/webp")
}

func (c *Client) upload(
	ctx context.Context,
	file []byte,
	md *CheckinMetadata,
	ext, contentType string,
) error {
	return c.putObject(ctx, CheckinKey(md.Date, md.ID, ext), file, md.ToMap(), contentType)
}

func (c *Client) putObject(
	ctx context.Context,
	key string,
	file []byte,
	metadata map[string]string,
	contentType string,
) error {
	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(file),
		Metadata:    metadata,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("failed to upload object %q: %w", key, err)
	}

	return nil
}

// Replace overwrites an existing object in place, preserving the given
// metadata (as returned by DownloadWithMetadata).
func (c *Client) Replace(
	ctx context.Context,
	key string,
	b []byte,
	md map[string]string,
	contentType string,
) error {
	return c.putObject(ctx, key, b, md, contentType)
}

// DownloadWithMetadata returns an object's bytes along with its user
// metadata, so the object can be rewritten in place without losing it.
func (c *Client) DownloadWithMetadata(
	ctx context.Context,
	key string,
) ([]byte, map[string]string, error) {
	output, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &c.bucketName,
		Key:    &key,
	})
	if err != nil {
		return nil, nil, err
	}
	defer output.Body.Close()

	b, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, nil, err
	}
	return b, output.Metadata, nil
}

// ListJPGKeys returns the keys of every .jpg object in the bucket.
func (c *Client) ListJPGKeys(ctx context.Context) ([]string, error) {
	var keys []string
	var continuationToken *string

	for {
		out, err := c.s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.bucketName),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range out.Contents {
			if obj.Key != nil && strings.HasSuffix(*obj.Key, ".jpg") {
				keys = append(keys, *obj.Key)
			}
		}

		if out.IsTruncated == nil || !*out.IsTruncated {
			return keys, nil
		}
		continuationToken = out.NextContinuationToken
	}
}

func (c *Client) Download(ctx context.Context, fileName string) ([]byte, error) {
	b, _, err := c.DownloadWithMetadata(ctx, fileName)
	return b, err
}

const latestKey = "latest.jpg"

func (c *Client) GetLatestCheckinID(ctx context.Context) (uint64, error) {
	const metaKeyID = "id"

	h, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(latestKey),
	})
	if err != nil {
		var nfe *types.NotFound
		if errors.As(err, &nfe) {
			log.Println("Latest key not found, starting from scratch")
			return 0, nil
		}
		return 0, fmt.Errorf("failed to head %q: %w", latestKey, err)
	}

	raw, ok := h.Metadata[metaKeyID]
	if !ok {
		return 0, fmt.Errorf(`missing "%s" metadata on %q`, metaKeyID, latestKey)
	}

	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf(`empty "%s" metadata on %q`, metaKeyID, latestKey)
	}

	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf(`invalid "%s" metadata value %q on %q: %w`, metaKeyID, s, latestKey, err)
	}

	log.Printf("Latest stored checkinID is: %d\n", id)
	return id, nil
}

func (c *Client) UpdateLatestCheckinID(
	ctx context.Context,
	checkinID uint64,
	createdAt time.Time,
) error {
	key := CheckinKey(createdAt, strconv.FormatUint(checkinID, 10), "jpg")
	copySource := c.bucketName + "/" + url.PathEscape(key)

	_, err := c.s3Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:            aws.String(c.bucketName),
		Key:               aws.String(latestKey),
		CopySource:        aws.String(copySource),
		MetadataDirective: types.MetadataDirectiveReplace,
		Metadata: map[string]string{
			"id":         strconv.FormatUint(checkinID, 10),
			"created_at": createdAt.Format(time.RFC3339),
		},
		ContentType: aws.String("image/jpeg"),
	})
	if err != nil {
		return fmt.Errorf("failed to copy %q to %q: %w", key, latestKey, err)
	}

	return nil
}

func (c *Client) CheckinExists(ctx context.Context, checkinID, createdAt string) (bool, error) {
	return c.checkinExists(ctx, checkinID, createdAt, "jpg")
}

func (c *Client) CheckinWEBPExists(ctx context.Context, checkinID, createdAt string) (bool, error) {
	return c.checkinExists(ctx, checkinID, createdAt, "webp")
}

func (c *Client) checkinExists(
	ctx context.Context,
	checkinID, createdAt, ext string,
) (bool, error) {
	t, err := time.Parse("2006-01-02 15:04:05", createdAt)
	if err != nil {
		return false, fmt.Errorf("parse checkin date %q: %w", createdAt, err)
	}

	key := CheckinKey(t, checkinID, ext)

	_, err = c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		var nfe *types.NotFound
		if errors.As(err, &nfe) {
			// object does not exist
			return false, nil
		}
		return false, fmt.Errorf("failed to head %q: %w", key, err)
	}

	return true, nil
}
