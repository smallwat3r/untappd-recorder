package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/smallwat3r/untappd-saver/internal/config"
	"github.com/smallwat3r/untappd-saver/internal/untappd"
)

type S3Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

type R2Client struct {
	cfg      *config.Config
	s3Client S3Client
}

func New(ctx context.Context, cfg *config.Config) (*R2Client, error) {
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID),
		}, nil
	})

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithEndpointResolverWithOptions(r2Resolver),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2AccessKeySecret, "")),
		awsconfig.WithRegion(cfg.R2Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return NewR2Client(cfg, s3.NewFromConfig(awsCfg)), nil
}

func generateKey(checkin untappd.Checkin) string {
	return fmt.Sprintf("%d.jpg", checkin.CheckinID)
}

func NewR2Client(cfg *config.Config, s3Client S3Client) *R2Client {
	return &R2Client{
		cfg:      cfg,
		s3Client: s3Client,
	}
}

func (c *R2Client) SaveCheckin(ctx context.Context, checkin untappd.Checkin) error {
	photoURL := ""
	if len(checkin.Media.Items) > 0 {
		photoURL = checkin.Media.Items[0].Photo.PhotoImgOg
	} else if checkin.Beer.BeerImage != "" {
		// fallback to using the photo used by the brewery for the beer
		photoURL = checkin.Beer.BeerImage
	}

	if photoURL == "" {
		return nil
	}

	fmt.Printf("Found photo: %s\n", photoURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photoURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for photo %s: %w", photoURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download photo %s: %w", photoURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download photo %s: status %s", photoURL, resp.Status)
	}

	photoBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read photo bytes: %w", err)
	}

	key := generateKey(checkin)
	_, err = c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &c.cfg.R2BucketName,
		Key:    &key,
		Body:   bytes.NewReader(photoBytes),
		Metadata: map[string]string{
			"id":      strconv.Itoa(checkin.CheckinID),
			"beer":    checkin.Beer.BeerName,
			"brewery": checkin.Brewery.BreweryName,
			"comment": checkin.CheckinComment,
			"rating":  fmt.Sprintf("%.2f", checkin.RatingScore),
			"venue":   checkin.Venue.VenueName,
			"date":    checkin.CreatedAt,
			"latlng":  fmt.Sprintf("%f,%f", checkin.Venue.Location.Lat, checkin.Venue.Location.Lng),
			"style":   checkin.Beer.BeerStyle,
			"abv":     fmt.Sprintf("%.2f", checkin.Beer.BeerABV),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to upload photo to R2: %w", err)
	}

	return nil
}

func (c *R2Client) GetLatestCheckinID(ctx context.Context) (int, error) {
	output, err := c.s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &c.cfg.R2BucketName,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list objects from R2: %w", err)
	}

	if len(output.Contents) == 0 {
		return 0, nil
	}

	latest := output.Contents[0]
	for _, obj := range output.Contents {
		if obj.LastModified.After(*latest.LastModified) {
			latest = obj
		}
	}

	headObj, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &c.cfg.R2BucketName,
		Key:    latest.Key,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get object metadata from R2: %w", err)
	}

	checkinID, err := strconv.Atoi(headObj.Metadata["id"])
	if err != nil {
		return 0, fmt.Errorf("failed to parse checkin ID from metadata: %w", err)
	}

	fmt.Printf("Latest stored checkinID is: %d\n", checkinID)
	return checkinID, nil
}
