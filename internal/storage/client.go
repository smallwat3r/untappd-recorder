package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/untappd"
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

	r2Resolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, _ ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               endpoint,
				HostnameImmutable: true,
				SigningRegion:     "auto",
				SigningName:       "s3",
			}, nil
		},
	)

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithEndpointResolverWithOptions(r2Resolver),
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

func (c *Client) Upload(ctx context.Context, file []byte, md *CheckinMetadata) error {
	t, err := time.Parse(time.RFC1123Z, md.Date)
	if err != nil {
		return fmt.Errorf("parse checkin date %q: %w", md.Date, err)
	}

	// YYYY/MM/DD/id.jpg
	key := path.Join(
		t.Format("2006/01/02"),
		fmt.Sprintf("%s.jpg", md.ID),
	)

	_, err = c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(file),
		Metadata:    md.ToMap(),
		ContentType: aws.String("image/jpeg"),
	})
	if err != nil {
		return fmt.Errorf("failed to upload object %q: %w", key, err)
	}

	return nil
}

func (c *Client) Download(ctx context.Context, fileName string) ([]byte, error) {
	output, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &c.bucketName,
		Key:    &fileName,
	})
	if err != nil {
		return nil, err
	}
	defer output.Body.Close()
	return io.ReadAll(output.Body)
}

func (c *Client) HeadObject(
	ctx context.Context,
	params *s3.HeadObjectInput,
	optFns ...func(*s3.Options),
) (*s3.HeadObjectOutput, error) {
	return c.s3Client.HeadObject(ctx, params, optFns...)
}

func (c *Client) ListObjectsV2(
	ctx context.Context,
	params *s3.ListObjectsV2Input,
	optFns ...func(*s3.Options),
) (*s3.ListObjectsV2Output, error) {
	return c.s3Client.ListObjectsV2(ctx, params, optFns...)
}

func (c *Client) CopyObject(
	ctx context.Context,
	params *s3.CopyObjectInput,
	optFns ...func(*s3.Options),
) (*s3.CopyObjectOutput, error) {
	return c.s3Client.CopyObject(ctx, params, optFns...)
}

func (c *Client) GetLatestCheckinID(ctx context.Context) (uint64, error) {
	const latestKey = "latest.jpg"
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

func (c *Client) UpdateLatestCheckinID(ctx context.Context, checkin untappd.Checkin) error {
	t, err := time.Parse(time.RFC1123Z, checkin.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to parse checkin date %q: %w", checkin.CreatedAt, err)
	}

	dir := t.Format("2006/01/02")
	key := path.Join(dir, fmt.Sprintf("%d.jpg", checkin.CheckinID))
	latestKey := "latest.jpg"

	copySource := c.bucketName + "/" + url.PathEscape(key)

	_, err = c.s3Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:            aws.String(c.bucketName),
		Key:               aws.String(latestKey),
		CopySource:        aws.String(copySource),
		MetadataDirective: types.MetadataDirectiveReplace,
		Metadata: map[string]string{
			"id":         strconv.FormatUint(checkin.CheckinID, 10),
			"created_at": t.Format(time.RFC3339),
		},
		ContentType: aws.String("image/jpeg"),
	})
	if err != nil {
		return fmt.Errorf("failed to copy %q to %q: %w", key, latestKey, err)
	}

	return nil
}

func (c *Client) CheckinExists(ctx context.Context, checkinID, createdAt string) (bool, error) {
	t, err := time.Parse("2006-01-02 15:04:05", createdAt)
	if err != nil {
		return false, fmt.Errorf("parse checkin date %q: %w", createdAt, err)
	}

	// YYYY/MM/DD/id.jpg
	key := path.Join(
		t.Format("2006/01/02"),
		fmt.Sprintf("%s.jpg", checkinID),
	)

	_, err = c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		var nfe *types.NotFound
		if errors.As(err, &nfe) {
			// object does not exists
			return false, nil
		}
		return false, fmt.Errorf("failed to head %q: %w", key, err)
	}

	return true, nil
}
