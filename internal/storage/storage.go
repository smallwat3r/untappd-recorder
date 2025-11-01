package storage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Storage interface {
	Upload(ctx context.Context, file []byte, fileName string, metadata map[string]string) error
	Download(ctx context.Context, fileName string) ([]byte, error)
}

type S3Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
}

// holds the metadata for a checkin photo
type CheckinMetadata struct {
	ID      string
	Beer    string
	Brewery string
	Comment string
	Rating  string
	Venue   string
	Date    string
	LatLng  string
	Style   string
	ABV     string
}

func (m *CheckinMetadata) ToMap() map[string]string {
	return map[string]string{
		"id":      m.ID,
		"beer":    m.Beer,
		"brewery": m.Brewery,
		"comment": m.Comment,
		"rating":  m.Rating,
		"venue":   m.Venue,
		"date":    m.Date,
		"latlng":  m.LatLng,
		"style":   m.Style,
		"abv":     m.ABV,
	}
}
