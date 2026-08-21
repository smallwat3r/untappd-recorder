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
	"time"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/photo"
	"github.com/smallwat3r/untappd-recorder/internal/processor"
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
	log.Println("Backfill completed successfully.")
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
		downloader = photo.NewDownloader(cfg.BlurFaces, float32(cfg.BlurMinQuality))
	}

	log.Printf("Starting backfill from %s\n", csvPath)
	return runBackfill(ctx, csvPath, store, cfg, downloader)
}

// the columns of the Untappd CSV export that we actually use.
type CSVRecord struct {
	BeerName       string
	BreweryName    string
	BeerType       string
	BeerABV        string
	Comment        string
	VenueName      string
	VenueCity      string
	VenueState     string
	VenueCountry   string
	VenueLat       string
	VenueLng       string
	RatingScore    string
	CreatedAt      string
	BreweryCountry string
	CheckinID      string
	PhotoURL       string
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
	processor.Process(ctx, records, cfg.NumWorkers, func(ctx context.Context, rec []string) {
		csvRecord, err := recordToCSVRecord(rec, header)
		if err != nil {
			log.Printf("error mapping record -> CSVRecord: %v", err)
			return
		}

		checkinID, err := strconv.ParseUint(csvRecord.CheckinID, 10, 64)
		if err != nil {
			log.Printf("invalid checkin ID %q: %v", csvRecord.CheckinID, err)
			return
		}

		log.Printf("Processing checkin %d", checkinID)

		exists, err := store.CheckinExists(ctx, csvRecord.CheckinID, csvRecord.CreatedAt)
		if err != nil {
			log.Printf("failed checking exists(%d): %v", checkinID, err)
			return
		}

		if exists {
			webpExists, err := store.CheckinWEBPExists(ctx, csvRecord.CheckinID, csvRecord.CreatedAt)
			if err != nil {
				log.Printf("failed checking webp exists(%d): %v", checkinID, err)
				return
			}
			if webpExists {
				log.Printf("checkin %d webp exists, skipping", checkinID)
				return
			}

			log.Printf("Backfilling WEBP for checkin %d", checkinID)
			if err := saveWEBPFromJPG(ctx, store, cfg, csvRecord, downloader); err != nil {
				log.Printf("failed to save webp(%d): %v", checkinID, err)
			}
			return
		}

		log.Printf("Backfilling checkin %d", checkinID)
		if err := saveCSVRecord(ctx, store, cfg, csvRecord, downloader); err != nil {
			log.Printf("failed to save(%d): %v", checkinID, err)
		}
	})
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
		BeerName:       recordMap["beer_name"],
		BreweryName:    recordMap["brewery_name"],
		BeerType:       recordMap["beer_type"],
		BeerABV:        recordMap["beer_abv"],
		Comment:        recordMap["comment"],
		VenueName:      recordMap["venue_name"],
		VenueCity:      recordMap["venue_city"],
		VenueState:     recordMap["venue_state"],
		VenueCountry:   recordMap["venue_country"],
		VenueLat:       recordMap["venue_lat"],
		VenueLng:       recordMap["venue_lng"],
		RatingScore:    recordMap["rating_score"],
		CreatedAt:      recordMap["created_at"],
		BreweryCountry: recordMap["brewery_country"],
		CheckinID:      recordMap["checkin_id"],
		PhotoURL:       recordMap["photo_url"],
	}, nil
}

func formatLatLng(record *CSVRecord) string {
	if record.VenueLat == "" || record.VenueLng == "" {
		return ""
	}
	return fmt.Sprintf("%s,%s", record.VenueLat, record.VenueLng)
}

func saveRecord(
	ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	record *CSVRecord,
	downloader photo.Downloader,
	saveWEBP bool,
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
		City:           untappd.BlankIfAtHome(record.VenueCity, record.VenueName),
		State:          untappd.BlankIfAtHome(record.VenueState, record.VenueName),
		Country:        untappd.BlankIfAtHome(record.VenueCountry, record.VenueName),
		LatLng:         untappd.BlankIfAtHome(formatLatLng(record), record.VenueName),
		Date:           createdAt,
		Style:          record.BeerType,
		ABV:            record.BeerABV,
	}

	if saveWEBP {
		return downloader.DownloadAndSaveWEBP(ctx, store, metadata)
	}
	return downloader.DownloadAndSave(
		ctx,
		store,
		record.PhotoURL,
		cfg.PlaceholderPhotoPath,
		metadata,
	)
}

func saveCSVRecord(ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	record *CSVRecord,
	downloader photo.Downloader,
) error {
	return saveRecord(ctx, store, cfg, record, downloader, false)
}

func saveWEBPFromJPG(
	ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	record *CSVRecord,
	downloader photo.Downloader,
) error {
	return saveRecord(ctx, store, cfg, record, downloader, true)
}
