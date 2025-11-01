package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
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
}

type R2Client struct {
	cfg      *config.Config
	s3Client S3Client
}

func New(cfg *config.Config) *R2Client {
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID),
		}, nil
	})

	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithEndpointResolverWithOptions(r2Resolver),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2AccessKeySecret, "")),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	return NewR2Client(cfg, s3.NewFromConfig(awsCfg))
}

func NewR2Client(cfg *config.Config, s3Client S3Client) *R2Client {
	return &R2Client{
		cfg:      cfg,
		s3Client: s3Client,
	}
}

func (c *R2Client) SaveCheckin(checkin untappd.Checkin) {
	if len(checkin.Media.Items) == 0 {
		return
	}

	photoURL := checkin.Media.Items[0].Photo.PhotoImgOg
	fmt.Printf("Found photo: %s\n", photoURL)

	resp, err := http.Get(photoURL)
	if err != nil {
		log.Printf("Failed to download photo %s: %v", photoURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to download photo %s: status %s", photoURL, resp.Status)
		return
	}

	photoBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read photo bytes: %v", err)
		return
	}

	key := fmt.Sprintf("%d.jpg", checkin.CheckinID)
	_, err = c.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: &c.cfg.R2BucketName,
		Key:    &key,
		Body:   bytes.NewReader(photoBytes),
		Metadata: map[string]string{
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
		log.Printf("Failed to upload photo to R2: %v", err)
	} else {
		fmt.Printf("Successfully uploaded %s to R2\n", key)
	}
}

func (c *R2Client) GetLatestCheckinID() (int, error) {
	output, err := c.s3Client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
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

	// The key is the checkin ID + .jpg, so we remove the extension.
	checkinID, err := strconv.Atoi((*latest.Key)[:len(*latest.Key)-4])
	if err != nil {
		return 0, fmt.Errorf("failed to parse checkin ID from key %s: %w", *latest.Key, err)
	}

	return checkinID, nil
}
