package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/smallwat3r/untappd-recorder/internal/processor"
)

// index.json lives at the bucket root and lists every WEBP photo with its
// decoded metadata, so the gallery frontend can filter check-ins without
// per-object HeadObject calls. It is derived data, the bucket objects stay
// the single source of truth, and RebuildManifest is the repair path.
const manifestKey = "index.json"

// the manifest changes on every upload, so it must not be cached for long
const manifestCacheControl = "public, max-age=300"

// ManifestRecord is one WEBP photo in index.json. Every field is always
// present (empty string when unknown) so consumers need no null checks.
type ManifestRecord struct {
	Key            string `json:"key"`
	ID             string `json:"id"`
	Beer           string `json:"beer"`
	Brewery        string `json:"brewery"`
	BreweryCountry string `json:"brewery_country"`
	Comment        string `json:"comment"`
	Rating         string `json:"rating"`
	Venue          string `json:"venue"`
	City           string `json:"city"`
	State          string `json:"state"`
	Country        string `json:"country"`
	LatLng         string `json:"latlng"`
	Date           string `json:"date"`
	Style          string `json:"style"`
	ABV            string `json:"abv"`
}

// manifestRecord builds a record from stored object metadata (as returned by
// HeadObject), decoding the header-safe encoding back to plain UTF-8.
func manifestRecord(key string, md map[string]string) ManifestRecord {
	get := func(k string) string { return decodeMetadataValue(md[k]) }
	return ManifestRecord{
		Key:            key,
		ID:             get("id"),
		Beer:           get("beer"),
		Brewery:        get("brewery"),
		BreweryCountry: get("brewery_country"),
		Comment:        get("comment"),
		Rating:         get("rating"),
		Venue:          get("venue"),
		City:           get("city"),
		State:          get("state"),
		Country:        get("country"),
		LatLng:         get("latlng"),
		Date:           get("date"),
		Style:          get("style"),
		ABV:            get("abv"),
	}
}

// decodeMetadataValue reverses the header-safe encoding applied on upload:
// RFC 2047 encoded words first, then percent-encoding. Values that fail to
// decode (a bare literal %) are kept as is.
func decodeMetadataValue(s string) string {
	if strings.Contains(s, "=?") {
		dec := mime.WordDecoder{}
		if decoded, err := dec.DecodeHeader(s); err == nil {
			s = decoded
		}
	}
	if strings.Contains(s, "%") {
		if decoded, err := url.PathUnescape(s); err == nil {
			s = decoded
		}
	}
	return s
}

// recordManifestEntry queues a freshly uploaded WEBP for the next
// UpdateManifest call. The record goes through the same sanitise/decode
// round trip as a rebuild, so both paths produce identical values.
func (c *Client) recordManifestEntry(key string, md *CheckinMetadata) {
	r := manifestRecord(key, md.ToMap())
	c.pendingMu.Lock()
	c.pending = append(c.pending, r)
	c.pendingMu.Unlock()
}

// UpdateManifest merges the records queued by this run's WEBP uploads into
// index.json. It is a no-op when nothing was uploaded. Records are keyed by
// object key, so re-processing a checkin updates its entry rather than
// duplicating it, and a single PUT keeps the file atomic for readers.
func (c *Client) UpdateManifest(ctx context.Context) error {
	c.pendingMu.Lock()
	pending := c.pending
	c.pendingMu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	existing, err := c.downloadManifest(ctx)
	if err != nil {
		return err
	}

	merged := mergeManifest(existing, pending)
	if err := c.putManifest(ctx, merged); err != nil {
		return err
	}

	// only clear the queue once the manifest write succeeded, so a failed
	// attempt can be retried without losing the records
	c.pendingMu.Lock()
	c.pending = c.pending[len(pending):]
	c.pendingMu.Unlock()

	log.Printf("Manifest updated: %d records (%d new this run)", len(merged), len(pending))
	return nil
}

// RebuildManifest regenerates index.json from scratch by walking every WEBP
// object in the bucket and reading its metadata. It returns the number of
// records written.
func (c *Client) RebuildManifest(ctx context.Context, numWorkers int) (int, error) {
	keys, err := c.listKeys(ctx, func(key string) bool {
		return strings.Contains(key, "/WEBP/") && strings.HasSuffix(key, ".webp")
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list WEBP objects: %w", err)
	}

	var (
		mu      sync.Mutex
		records []ManifestRecord
		errs    []error
	)
	processor.Process(ctx, keys, numWorkers, func(ctx context.Context, key string) {
		h, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(c.bucketName),
			Key:    aws.String(key),
		})
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to head %q: %w", key, err))
			return
		}
		records = append(records, manifestRecord(key, h.Metadata))
	})
	if len(errs) > 0 {
		return 0, fmt.Errorf("%d objects could not be read, first error: %w", len(errs), errs[0])
	}

	sortManifest(records)
	if err := c.putManifest(ctx, records); err != nil {
		return 0, err
	}

	// the manifest is only trustworthy if it covers every WEBP object
	if len(records) != len(keys) {
		return len(records), fmt.Errorf(
			"manifest has %d records but bucket has %d WEBP objects", len(records), len(keys),
		)
	}
	return len(records), nil
}

func (c *Client) downloadManifest(ctx context.Context) ([]ManifestRecord, error) {
	b, _, err := c.DownloadWithMetadata(ctx, manifestKey)
	if err != nil {
		var noKey *types.NoSuchKey
		var notFound *types.NotFound
		if errors.As(err, &noKey) || errors.As(err, &notFound) {
			log.Printf("No %s yet, starting a fresh manifest", manifestKey)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to download %q: %w", manifestKey, err)
	}

	var records []ManifestRecord
	if err := json.Unmarshal(b, &records); err != nil {
		return nil, fmt.Errorf(
			"failed to parse %q (run the manifest rebuild command to repair it): %w",
			manifestKey, err,
		)
	}
	return records, nil
}

func (c *Client) putManifest(ctx context.Context, records []ManifestRecord) error {
	// an empty manifest is still a valid JSON array, never "null"
	if records == nil {
		records = []ManifestRecord{}
	}
	b, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("failed to encode manifest: %w", err)
	}

	_, err = c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(c.bucketName),
		Key:          aws.String(manifestKey),
		Body:         bytes.NewReader(b),
		ContentType:  aws.String("application/json"),
		CacheControl: aws.String(manifestCacheControl),
	})
	if err != nil {
		return fmt.Errorf("failed to upload %q: %w", manifestKey, err)
	}
	return nil
}

// mergeManifest overlays new records on the existing ones by object key and
// returns them sorted newest first.
func mergeManifest(existing, updates []ManifestRecord) []ManifestRecord {
	byKey := make(map[string]ManifestRecord, len(existing)+len(updates))
	for _, r := range existing {
		byKey[r.Key] = r
	}
	for _, r := range updates {
		byKey[r.Key] = r
	}

	merged := make([]ManifestRecord, 0, len(byKey))
	for _, r := range byKey {
		merged = append(merged, r)
	}
	sortManifest(merged)
	return merged
}

// sortManifest orders records newest first by date, with the key as a
// deterministic tie-break. Unparseable dates sort last.
func sortManifest(records []ManifestRecord) {
	parse := func(s string) time.Time {
		t, err := time.Parse(time.RFC1123Z, s)
		if err != nil {
			return time.Time{}
		}
		return t
	}
	sort.Slice(records, func(i, j int) bool {
		ti, tj := parse(records[i].Date), parse(records[j].Date)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return records[i].Key > records[j].Key
	})
}
