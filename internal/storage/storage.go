package storage

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Storage interface {
	UploadJPG(ctx context.Context, file []byte, metadata *CheckinMetadata) error
	UploadWEBP(ctx context.Context, file []byte, metadata *CheckinMetadata) error
	Download(ctx context.Context, fileName string) ([]byte, error)
	CheckinExists(ctx context.Context, checkinID, createdAt string) (bool, error)
	CheckinWEBPExists(ctx context.Context, checkinID, createdAt string) (bool, error)
	GetLatestCheckinID(ctx context.Context) (uint64, error)
	UpdateLatestCheckinID(ctx context.Context, checkinID uint64, createdAt time.Time) error
}

type S3Client interface {
	PutObject(
		ctx context.Context,
		params *s3.PutObjectInput,
		optFns ...func(*s3.Options),
	) (*s3.PutObjectOutput, error)
	GetObject(
		ctx context.Context,
		params *s3.GetObjectInput,
		optFns ...func(*s3.Options),
	) (*s3.GetObjectOutput, error)
	ListObjectsV2(
		ctx context.Context,
		params *s3.ListObjectsV2Input,
		optFns ...func(*s3.Options),
	) (*s3.ListObjectsV2Output, error)
	HeadObject(
		ctx context.Context,
		params *s3.HeadObjectInput,
		optFns ...func(*s3.Options),
	) (*s3.HeadObjectOutput, error)
	CopyObject(
		ctx context.Context,
		params *s3.CopyObjectInput,
		optFns ...func(*s3.Options),
	) (*s3.CopyObjectOutput, error)
}

// CheckinKey returns the object key for a checkin photo:
// YYYY/MM/DD/id.jpg for JPGs, YYYY/MM/DD/WEBP/id.webp for WebPs.
func CheckinKey(t time.Time, id, ext string) string {
	dir := t.Format("2006/01/02")
	if ext == "webp" {
		dir = path.Join(dir, "WEBP")
	}
	return path.Join(dir, fmt.Sprintf("%s.%s", id, ext))
}

// WEBPSiblingKey maps a JPG key to its WebP sibling, mirroring CheckinKey's
// layout (YYYY/MM/DD/id.jpg -> YYYY/MM/DD/WEBP/id.webp). Keys without a
// date directory (latest.jpg) have no sibling and map to "".
func WEBPSiblingKey(key string) string {
	dir, file := path.Split(key)
	if dir == "" {
		return ""
	}
	return path.Join(dir, "WEBP", strings.TrimSuffix(file, ".jpg")+".webp")
}

// holds the metadata for a checkin photo
type CheckinMetadata struct {
	ID             string
	Beer           string
	Brewery        string
	BreweryCountry string
	Comment        string
	Rating         string
	Venue          string
	City           string
	State          string
	Country        string
	LatLng         string
	Date           time.Time
	Style          string
	ABV            string
}

func (m *CheckinMetadata) ToMap() map[string]string {
	out := map[string]string{
		"id":              m.ID,
		"beer":            m.Beer,
		"brewery":         m.Brewery,
		"brewery_country": m.BreweryCountry,
		"comment":         m.Comment,
		"rating":          m.Rating,
		"venue":           m.Venue,
		"city":            m.City,
		"state":           m.State,
		"country":         m.Country,
		"latlng":          m.LatLng,
		"date":            m.Date.Format(time.RFC1123Z),
		"style":           m.Style,
		"abv":             m.ABV,
	}
	for k, v := range out {
		out[k] = sanitizeMetadataValue(v)
	}
	return out
}

// S3/R2 user metadata values must be single-line US-ASCII, and the total
// metadata size is capped (~2KB). Beer names, breweries and comments can carry
// accents, emoji or newlines, which would otherwise break the request signing
// or get silently dropped. We collapse line breaks, strip other control
// characters, cap the length, then percent-encode any non-ASCII byte so the
// value stays header safe and reversible while ASCII text remains readable.
const maxMetadataValueBytes = 512

const upperHex = "0123456789ABCDEF"

func sanitizeMetadataValue(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, s)

	s = strings.TrimSpace(s)
	if len(s) > maxMetadataValueBytes {
		s = truncateUTF8(s, maxMetadataValueBytes)
	}

	return percentEncode(s)
}

func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	// if the cut lands inside a multi-byte rune, back up to that rune's
	// start so we never emit a partial rune.
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func percentEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c <= 0x7e && c != '%' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperHex[c>>4])
		b.WriteByte(upperHex[c&0x0f])
	}
	return b.String()
}
