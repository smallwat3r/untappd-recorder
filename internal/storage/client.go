package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
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
	if cfg.R2AccountID != "" {
		return newR2Client(ctx, cfg)
	}
	if cfg.AWSRegion != "" {
		return newS3Client(ctx, cfg)
	}
	return nil, fmt.Errorf("no storage provider configured")
}

func newR2Client(ctx context.Context, cfg *config.Config) (*Client, error) {
	r2Resolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL: fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID),
			}, nil
		},
	)

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithEndpointResolverWithOptions(r2Resolver),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2AccessKeySecret, ""),
		),
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

func (c *Client) GetLatestCheckinID(ctx context.Context) (int, error) {
	headObj, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &c.bucketName,
		Key:    aws.String("latest.jpg"),
	})
	if err != nil {
		var nfe *types.NotFound
		if errors.As(err, &nfe) {
			fmt.Println("latest key not found, starting from scratch")
			return 0, nil
		}
		return 0, fmt.Errorf("failed to head latest key: %w", err)
	}

	checkinID, err := strconv.Atoi(headObj.Metadata["id"])
	if err != nil {
		return 0, fmt.Errorf("failed to parse checkin ID from metadata: %w", err)
	}

	fmt.Printf("Latest stored checkinID is: %d\n", checkinID)
	return checkinID, nil
}

// sets the 'latest' key to alias the most recent check-in image.
// the original image (named by its ID) is preserved 'latest' is simply overwritten.
func (c *Client) UpdateLatestCheckinID(ctx context.Context, checkin untappd.Checkin) error {
	checkinTime, err := time.Parse(time.RFC1123Z, checkin.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to parse checkin date %s: %w", checkin.CreatedAt, err)
	}

	year := checkinTime.Format("2006")
	month := checkinTime.Format("01")
	day := checkinTime.Format("02")

	key := path.Join(year, month, day, fmt.Sprintf("%s.jpg", strconv.Itoa(checkin.CheckinID)))
	latestKey := "latest.jpg"

	sourceKey := path.Join(c.bucketName, key)
	_, err = c.s3Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     &c.bucketName,
		CopySource: aws.String(sourceKey),
		Key:        &latestKey,
		Metadata: map[string]string{
			"id": strconv.Itoa(checkin.CheckinID),
		},
		MetadataDirective: types.MetadataDirectiveReplace,
	})
	return err
}

func (c *Client) CheckinExists(ctx context.Context, checkinID, createdAt string) (bool, error) {
	checkinTime, err := time.Parse("2006-01-02 15:04:05", createdAt)
	if err != nil {
		return false, fmt.Errorf("failed to parse checkin date %s: %w", createdAt, err)
	}

	year := checkinTime.Format("2006")
	month := checkinTime.Format("01")
	day := checkinTime.Format("02")

	key := path.Join(year, month, day, fmt.Sprintf("%s.jpg", checkinID))

	_, err = c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &c.bucketName,
		Key:    aws.String(key),
	})
	if err != nil {
		var nfe *types.NotFound
		if errors.As(err, &nfe) {
			return false, nil
		}
		return false, fmt.Errorf("failed to head object: %w", err)
	}

	return true, nil
}
