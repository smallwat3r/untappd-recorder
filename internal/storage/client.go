package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/smallwat3r/untappd-saver/internal/config"
)

// implements the Storage interface for S3-compatible services.
type Client struct {
	s3Client   S3Client
	bucketName string
}

func NewClient(ctx context.Context, cfg *config.Config) (*Client, error) {
	if cfg.R2AccountID != "" {
		return newR2Client(ctx, cfg)
	}
	if cfg.AWSRegion != "" {
		return newS3Client(ctx, cfg)
	}
	return nil, fmt.Errorf("no storage provider configured")
}

func newR2Client(ctx context.Context, cfg *config.Config) (*Client, error) {
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID),
		}, nil
	})

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithEndpointResolverWithOptions(r2Resolver),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2AccessKeySecret, "")),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for R2: %w", err)
	}

	return &Client{
		s3Client:   s3.NewFromConfig(awsCfg),
		bucketName: cfg.BucketName,
	}, nil
}

func newS3Client(ctx context.Context, cfg *config.Config) (*Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for S3: %w", err)
	}

	return &Client{
		s3Client:   s3.NewFromConfig(awsCfg),
		bucketName: cfg.BucketName,
	}, nil
}

func (c *Client) Upload(ctx context.Context, file []byte, metadata *CheckinMetadata) error {
	checkinTime, err := time.Parse(time.RFC1123Z, metadata.Date)
	if err != nil {
		return fmt.Errorf("failed to parse checkin date %s: %w", metadata.Date, err)
	}

	year := checkinTime.Format("2006")
	month := checkinTime.Format("01")
	day := checkinTime.Format("02")

	datedKey := path.Join(year, month, day, fmt.Sprintf("%s.jpg", metadata.ID))

	_, err = c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:   &c.bucketName,
		Key:      &datedKey,
		Body:     bytes.NewReader(file),
		Metadata: metadata.ToMap(),
	})
	if err != nil {
		return fmt.Errorf("failed to upload dated object: %w", err)
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

func (c *Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return c.s3Client.HeadObject(ctx, params, optFns...)
}

func (c *Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return c.s3Client.ListObjectsV2(ctx, params, optFns...)
}

func (c *Client) CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	return c.s3Client.CopyObject(ctx, params, optFns...)
}
