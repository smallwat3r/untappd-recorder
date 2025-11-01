package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/photo"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	store, err := storage.NewClient(ctx, cfg)
	if err != nil {
		log.Fatalf("Error creating storage client: %v", err)
	}

	csvPath := flag.String("csv", "", "path to a CSV file to backfill from")
	flag.Parse()

	if *csvPath == "" {
		log.Fatal("-csv is required for backfill command")
	}

	fmt.Printf("Starting backfill from %s\n", *csvPath)
	if err := runBackfill(ctx, *csvPath, store, cfg); err != nil {
		log.Fatalf("Backfill failed: %v", err)
	}
	fmt.Println("Backfill completed successfully.")
}

// matches the structure of the Untappd CSV export.
type CSVRecord struct {
	BeerName                  string
	BreweryName               string
	BeerType                  string
	BeerABV                   string
	BeerIBU                   string
	Comment                   string
	VenueName                 string
	VenueCity                 string
	VenueState                string
	VenueCountry              string
	VenueLat                  string
	VenueLng                  string
	RatingScore               string
	CreatedAt                 string
	CheckinURL                string
	BeerURL                   string
	BreweryURL                string
	BreweryCountry            string
	BreweryCity               string
	BreweryState              string
	FlavorProfiles            string
	PurchaseVenue             string
	ServingType               string
	CheckinID                 string
	BID                       string
	BreweryID                 string
	PhotoURL                  string
	GlobalRatingScore         string
	GlobalWeightedRatingScore string
	TaggedFriends             string
	TotalToasts               string
	TotalComments             string
}

func runBackfill(ctx context.Context, csvPath string, store *storage.Client, cfg *config.Config) error {
	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("could not open csv file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("could not read csv header: %w", err)
	}

	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("could not read csv records: %w", err)
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)

	for _, record := range records {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(rec []string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			csvRecord, err := recordToCSVRecord(rec, header)
			if err != nil {
				log.Printf("Error mapping record to CSVRecord: %v", err)
				return
			}

			checkinID, err := strconv.Atoi(csvRecord.CheckinID)
			if err != nil {
				log.Printf("Invalid checkin ID %s: %v", csvRecord.CheckinID, err)
				return
			}

			log.Printf("Processing checkin %d", checkinID)

			exists, err := checkinExists(ctx, store, cfg, csvRecord)
			if err != nil {
				log.Printf("Error checking if checkin %d exists: %v", checkinID, err)
				return
			}

			if exists {
				log.Printf("Checkin %d already exists, skipping", checkinID)
				return
			}

			log.Printf("Backfilling checkin %d", checkinID)
			if err := saveCSVRecord(ctx, store, cfg, csvRecord); err != nil {
				log.Printf("Failed to save checkin %d: %v", checkinID, err)
			}
		}(record)
	}

	wg.Wait()
	return nil
}

func recordToCSVRecord(record []string, header []string) (*CSVRecord, error) {
	if len(record) != len(header) {
		return nil, fmt.Errorf("record length (%d) does not match header length (%d)", len(record), len(header))
	}

	recordMap := make(map[string]string)
	for i, h := range header {
		recordMap[h] = record[i]
	}

	return &CSVRecord{
		BeerName:                  recordMap["beer_name"],
		BreweryName:               recordMap["brewery_name"],
		BeerType:                  recordMap["beer_type"],
		BeerABV:                   recordMap["beer_abv"],
		BeerIBU:                   recordMap["beer_ibu"],
		Comment:                   recordMap["comment"],
		VenueName:                 recordMap["venue_name"],
		VenueCity:                 recordMap["venue_city"],
		VenueState:                recordMap["venue_state"],
		VenueCountry:              recordMap["venue_country"],
		VenueLat:                  recordMap["venue_lat"],
		VenueLng:                  recordMap["venue_lng"],
		RatingScore:               recordMap["rating_score"],
		CreatedAt:                 recordMap["created_at"],
		CheckinURL:                recordMap["checkin_url"],
		BeerURL:                   recordMap["beer_url"],
		BreweryURL:                recordMap["brewery_url"],
		BreweryCountry:            recordMap["brewery_country"],
		BreweryCity:               recordMap["brewery_city"],
		BreweryState:              recordMap["brewery_state"],
		FlavorProfiles:            recordMap["flavor_profiles"],
		PurchaseVenue:             recordMap["purchase_venue"],
		ServingType:               recordMap["serving_type"],
		CheckinID:                 recordMap["checkin_id"],
		BID:                       recordMap["bid"],
		BreweryID:                 recordMap["brewery_id"],
		PhotoURL:                  recordMap["photo_url"],
		GlobalRatingScore:         recordMap["global_rating_score"],
		GlobalWeightedRatingScore: recordMap["global_weighted_rating_score"],
		TaggedFriends:             recordMap["tagged_friends"],
		TotalToasts:               recordMap["total_toasts"],
		TotalComments:             recordMap["total_comments"],
	}, nil
}

func checkinExists(ctx context.Context, store *storage.Client, cfg *config.Config, record *CSVRecord) (bool, error) {
	checkinTime, err := time.Parse("2006-01-02 15:04:05", record.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("failed to parse checkin date %s: %w", record.CreatedAt, err)
	}

	year := checkinTime.Format("2006")
	month := checkinTime.Format("01")
	day := checkinTime.Format("02")

	key := path.Join(year, month, day, fmt.Sprintf("%s.jpg", record.CheckinID))

	_, err = store.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &cfg.BucketName,
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

func saveCSVRecord(ctx context.Context, store storage.Storage, cfg *config.Config, record *CSVRecord) error {
	createdAt, err := time.Parse("2006-01-02 15:04:05", record.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to parse created_at: %w", err)
	}

	metadata := &storage.CheckinMetadata{
		ID:           record.CheckinID,
		Beer:         record.BeerName,
		Brewery:      record.BreweryName,
		Comment:      record.Comment,
		Rating:       record.RatingScore,
		Venue:        record.VenueName,
		Date:         createdAt.Format(time.RFC1123Z),
		LatLng:       fmt.Sprintf("%s,%s", record.VenueLat, record.VenueLng),
		Style:        record.BeerType,
		ABV:          record.BeerABV,
		ServingStyle: record.ServingType,
	}

	return photo.DownloadAndSave(ctx, cfg, store, record.PhotoURL, metadata)
}
