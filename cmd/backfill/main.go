package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/photo"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
	"github.com/smallwat3r/untappd-recorder/internal/untappd"
)

func main() {
	csvPath := flag.String("csv", "", "path to a CSV file to backfill from")
	flag.Parse()

	if *csvPath == "" {
		log.Fatal("-csv is required for backfill command")
	}

	if err := run(context.Background(), *csvPath, nil, nil); err != nil {
		log.Fatalf("backfill failed: %v", err)
	}
	fmt.Println("backfill completed successfully.")
}

func run(
	ctx context.Context,
	csvPath string,
	store storage.Storage,
	downloader photo.Downloader,
) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}

	if store == nil {
		s, err := storage.NewClient(ctx, cfg)
		if err != nil {
			return fmt.Errorf("error creating storage client: %w", err)
		}
		store = s
	}

	if downloader == nil {
		downloader = photo.NewDownloader()
	}

	fmt.Printf("Starting backfill from %s\n", csvPath)
	return runBackfill(ctx, csvPath, store, cfg, downloader)
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

func runBackfill(
	ctx context.Context,
	csvPath string,
	store storage.Storage,
	cfg *config.Config,
	downloader photo.Downloader,
) error {
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
	// remove Byte Order Mark (BOM) if present, often found in CSV files
	header[0] = strings.TrimPrefix(header[0], "\ufeff")

	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("could not read csv records: %w", err)
	}

	processCSVRecords(ctx, store, cfg, records, header, downloader)
	return nil
}

func processCSVRecords(
	ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	records [][]string,
	header []string,
	downloader photo.Downloader,
) {
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
				log.Printf("error mapping record to CSVRecord: %v", err)
				return
			}

			checkinID, err := strconv.Atoi(csvRecord.CheckinID)
			if err != nil {
				log.Printf("invalid checkin ID %s: %v", csvRecord.CheckinID, err)
				return
			}

			log.Printf("processing checkin %d", checkinID)

			exists, err := store.CheckinExists(ctx, csvRecord.CheckinID, csvRecord.CreatedAt)
			if err != nil {
				log.Printf("error checking if checkin %d exists: %v", checkinID, err)
				return
			}

			if exists {
				log.Printf("checkin %d already exists, skipping", checkinID)
				return
			}

			log.Printf("backfilling checkin %d", checkinID)
			if err := saveCSVRecord(ctx, store, cfg, csvRecord, downloader); err != nil {
				log.Printf("failed to save checkin %d: %v", checkinID, err)
			}
		}(record)
	}

	wg.Wait()
}

func recordToCSVRecord(record []string, header []string) (*CSVRecord, error) {
	if len(record) != len(header) {
		return nil, fmt.Errorf(
			"record length (%d) does not match header length (%d)",
			len(record),
			len(header),
		)
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

func formatLatLng(record *CSVRecord) string {
	if record.VenueLat == "" || record.VenueLng == "" {
		return ""
	}
	return fmt.Sprintf("%s,%s", record.VenueLat, record.VenueLng)
}

func formatFromAtHomeVenue(value, venue string) string {
	if venue == untappd.VenueUntappdAtHome {
		return ""
	}
	return value
}

func saveCSVRecord(
	ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	record *CSVRecord,
	downloader photo.Downloader,
) error {
	createdAt, err := time.Parse("2006-01-02 15:04:05", record.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to parse created_at: %w", err)
	}

	metadata := &storage.CheckinMetadata{
		ID:             record.CheckinID,
		Beer:           record.BeerName,
		Brewery:        record.BreweryName,
		BreweryCountry: record.BreweryCountry,
		Comment:        record.Comment,
		Rating:         record.RatingScore,
		Venue:          record.VenueName,
		City:           formatFromAtHomeVenue(record.VenueCity, record.VenueName),
		State:          formatFromAtHomeVenue(record.VenueState, record.VenueName),
		Country:        formatFromAtHomeVenue(record.VenueCountry, record.VenueName),
		LatLng:         formatFromAtHomeVenue(formatLatLng(record), record.VenueName),
		Date:           createdAt.Format(time.RFC1123Z),
		Style:          record.BeerType,
		ABV:            record.BeerABV,
	}

	return downloader.DownloadAndSave(ctx, cfg, store, record.PhotoURL, metadata)
}
