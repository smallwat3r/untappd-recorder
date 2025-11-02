package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/photo"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
	"github.com/smallwat3r/untappd-recorder/internal/untappd"
)

func main() {
	if err := run(context.Background(), nil, nil); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Record completed successfully.")
}

func run(ctx context.Context, store storage.Storage, untappdClient untappd.UntappdClient) error {
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

	if untappdClient == nil {
		untappdClient = untappd.NewClient(cfg)
	}

	downloader := photo.NewDownloader()

	return runRecorder(ctx, store, cfg, untappdClient, downloader)
}

func runRecorder(ctx context.Context, store storage.Storage, cfg *config.Config, untappdClient untappd.UntappdClient, downloader photo.Downloader) error {
	latestCheckinIDKey, err := store.GetLatestCheckinID(ctx)
	if err != nil {
		return fmt.Errorf("error getting latest checkin ID: %w", err)
	}

	var once sync.Once
	return untappdClient.FetchCheckins(ctx, latestCheckinIDKey, func(ctx context.Context, checkins []untappd.Checkin) error {
		fmt.Printf("Processing %d checkins\n", len(checkins))
		processCheckins(ctx, store, cfg, checkins, downloader)

		// we set the first checkin to be the latest, so we remember from where
		// to start next time the script runs.
		if len(checkins) > 0 {
			once.Do(func() {
				if err := store.UpdateLatestCheckinID(ctx, checkins[0]); err != nil {
					log.Printf("Failed to update latest checkin ID: %v", err)
				}
			})
		}

		return nil
	})
}

func processCheckins(ctx context.Context, store storage.Storage, cfg *config.Config, checkins []untappd.Checkin, downloader photo.Downloader) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)

	for _, checkin := range checkins {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(c untappd.Checkin) {
			defer wg.Done()
			defer func() { <-semaphore }()

			log.Printf("Processing checkin %d", c.CheckinID)
			if err := saveCheckin(ctx, store, cfg, c, downloader); err != nil {
				log.Printf("Failed to save checkin %d: %v", c.CheckinID, err)
			}
		}(checkin)
	}

	wg.Wait()
}

func saveCheckin(ctx context.Context, store storage.Storage, cfg *config.Config, checkin untappd.Checkin, downloader photo.Downloader) error {
	photoURL := ""
	if len(checkin.Media.Items) > 0 {
		photoURL = checkin.Media.Items[0].Photo.PhotoImgOg
	}

	metadata := &storage.CheckinMetadata{
		ID:             strconv.Itoa(checkin.CheckinID),
		Beer:           checkin.Beer.BeerName,
		Brewery:        checkin.Brewery.BreweryName,
		BreweryCountry: checkin.Brewery.BreweryCountry,
		Comment:        checkin.CheckinComment,
		Rating:         fmt.Sprintf("%.2f", checkin.RatingScore),
		Venue:          checkin.Venue.Name(),
		City:           checkin.Venue.City(),
		State:          checkin.Venue.State(),
		Country:        checkin.Venue.Country(),
		LatLng:         checkin.Venue.LatLng(),
		Date:           checkin.CreatedAt,
		Style:          checkin.Beer.BeerStyle,
		ABV:            fmt.Sprintf("%.2f", checkin.Beer.BeerABV),
	}

	return downloader.DownloadAndSave(ctx, cfg, store, photoURL, metadata)
}
