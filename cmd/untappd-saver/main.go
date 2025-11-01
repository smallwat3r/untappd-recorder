package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/smallwat3r/untappd-saver/internal/config"
	"github.com/smallwat3r/untappd-saver/internal/storage"
	"github.com/smallwat3r/untappd-saver/internal/untappd"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	fmt.Println("Successfully loaded configuration.")

	store, err := storage.NewClient(ctx, cfg)
	if err != nil {
		log.Fatalf("Error creating storage client: %v", err)
	}

	untappdClient := untappd.NewClient(cfg)
	latestCheckinIDKey, err := getLatestCheckinIDKey(ctx, store, cfg)
	if err != nil {
		log.Fatalf("Error getting latest checkin ID: %v", err)
	}

	latestUpdated := false
	err = untappdClient.FetchCheckins(ctx, latestCheckinIDKey, func(ctx context.Context, checkins []untappd.Checkin) error {
		fmt.Printf("Processing %d checkins\n", len(checkins))

		var wg sync.WaitGroup
		semaphore := make(chan struct{}, 10)

		for _, checkin := range checkins {
			wg.Add(1)
			semaphore <- struct{}{}

			go func(c untappd.Checkin) {
				defer wg.Done()
				defer func() { <-semaphore }()

				log.Printf("Processing checkin %d", c.CheckinID)
				if err := saveCheckin(ctx, store, cfg, c); err != nil {
					log.Printf("Failed to save checkin %d: %v", c.CheckinID, err)
				}
			}(checkin)
		}

		wg.Wait()

		// we set the first checkin to be the latest (most recent to oldest), so we remember
		// from where to start next time the script runs.
		if len(checkins) > 0 && !latestUpdated {
			if err := updateLatestCheckinIDKey(ctx, store, cfg, checkins[0]); err != nil {
				log.Printf("Failed to update latest checkin ID: %v", err)
			}
			latestUpdated = true
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Error fetching checkins: %v", err)
	}
}

func saveCheckin(ctx context.Context, store storage.Storage, cfg *config.Config, checkin untappd.Checkin) error {
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

	metadata := storage.CheckinMetadata{
		ID:      strconv.Itoa(checkin.CheckinID),
		Beer:    checkin.Beer.BeerName,
		Brewery: checkin.Brewery.BreweryName,
		Comment: checkin.CheckinComment,
		Rating:  fmt.Sprintf("%.2f", checkin.RatingScore),
		Venue:   checkin.Venue.VenueName,
		Date:    checkin.CreatedAt,
		LatLng:  fmt.Sprintf("%f,%f", checkin.Venue.Location.Lat, checkin.Venue.Location.Lng),
		Style:   checkin.Beer.BeerStyle,
		ABV:     fmt.Sprintf("%.2f", checkin.Beer.BeerABV),
	}

	return store.Upload(ctx, photoBytes, &metadata)
}

func getLatestCheckinIDKey(ctx context.Context, store storage.Storage, cfg *config.Config) (int, error) {
	type headObjectClient interface {
		HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	}

	l, ok := store.(headObjectClient)
	if !ok {
		return 0, fmt.Errorf("storage provider does not support HeadObject")
	}

	headObj, err := l.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &cfg.BucketName,
		Key:    aws.String("latest"),
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
func updateLatestCheckinIDKey(ctx context.Context, store storage.Storage, cfg *config.Config, checkin untappd.Checkin) error {
	type copyObjectClient interface {
		CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	}

	c, ok := store.(copyObjectClient)
	if !ok {
		return fmt.Errorf("storage provider does not support CopyObject")
	}

	checkinTime, err := time.Parse(time.RFC1123Z, checkin.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to parse checkin date %s: %w", checkin.CreatedAt, err)
	}

	year := checkinTime.Format("2006")
	month := checkinTime.Format("01")
	day := checkinTime.Format("02")

	key := path.Join(year, month, day, fmt.Sprintf("%s.jpg", strconv.Itoa(checkin.CheckinID)))
	latestKey := "latest.jpg"

	sourceKey := path.Join(cfg.BucketName, key)
	_, err = c.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     &cfg.BucketName,
		CopySource: aws.String(sourceKey),
		Key:        &latestKey,
	})
	return err
}
