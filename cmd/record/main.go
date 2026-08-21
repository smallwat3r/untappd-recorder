package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/photo"
	"github.com/smallwat3r/untappd-recorder/internal/processor"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
	"github.com/smallwat3r/untappd-recorder/internal/untappd"
)

func main() {
	if err := run(context.Background(), nil, nil); err != nil {
		log.Fatalf("record failed: %v", err)
	}
	log.Println("Record completed successfully.")
}

func run(ctx context.Context, store storage.Storage, untappdClient untappd.UntappdClient) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}
	if cfg.UntappdAccessToken == "" {
		return fmt.Errorf("UNTAPPD_ACCESS_TOKEN must be set")
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

	downloader := photo.NewDownloader(cfg.BlurFaces, float32(cfg.BlurMinQuality))

	return runRecorder(ctx, store, cfg, untappdClient, downloader)
}

func runRecorder(
	ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	untappdClient untappd.UntappdClient,
	downloader photo.Downloader,
) error {
	latestCheckinID, err := store.GetLatestCheckinID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest checkin ID: %w", err)
	}

	var newest *untappd.Checkin
	var failed int64

	proc := func(ctx context.Context, checkins []untappd.Checkin) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if len(checkins) == 0 {
			log.Printf("No checkins to process\n")
			return nil
		}

		log.Printf("Processing %d checkins\n", len(checkins))
		failed += processCheckins(ctx, store, cfg, checkins, downloader)

		if newest == nil {
			// first element of the first page is the newest checkin
			c := checkins[0]
			newest = &c
		}
		return nil
	}

	if err := untappdClient.FetchCheckins(ctx, latestCheckinID, proc); err != nil {
		return err
	}

	if newest == nil {
		return nil
	}

	// publish whatever uploaded successfully; if this fails the marker below
	// does not advance, so the next run re-uploads and retries the manifest
	if err := store.UpdateManifest(ctx); err != nil {
		return fmt.Errorf("failed to update manifest: %w", err)
	}

	// only advance the marker when every checkin saved, so failed ones are
	// retried on the next run instead of being skipped forever
	if failed > 0 {
		return fmt.Errorf(
			"%d checkins failed, keeping latest checkin marker at %d so they are retried",
			failed, latestCheckinID,
		)
	}
	return updateLatestCheckinID(ctx, store, *newest)
}

func updateLatestCheckinID(
	ctx context.Context,
	store storage.Storage,
	checkin untappd.Checkin,
) error {
	createdAt, err := parseCreatedAt(checkin.CreatedAt)
	if err != nil {
		return err
	}
	return store.UpdateLatestCheckinID(ctx, checkin.CheckinID, createdAt)
}

func parseCreatedAt(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC1123Z, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse checkin date %q: %w", s, err)
	}
	return t, nil
}

// processCheckins saves the checkins concurrently and returns how many
// failed.
func processCheckins(
	ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	checkins []untappd.Checkin,
	downloader photo.Downloader,
) int64 {
	var failed atomic.Int64
	processor.Process(ctx, checkins, cfg.NumWorkers, func(
		ctx context.Context,
		c untappd.Checkin,
	) {
		log.Printf("Processing checkin %d", c.CheckinID)
		if err := saveCheckin(ctx, store, cfg, c, downloader); err != nil {
			failed.Add(1)
			log.Printf("failed to save checkin %d: %v", c.CheckinID, err)
		}
	})
	return failed.Load()
}

func saveCheckin(
	ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	checkin untappd.Checkin,
	downloader photo.Downloader,
) error {
	createdAt, err := parseCreatedAt(checkin.CreatedAt)
	if err != nil {
		return err
	}

	photoURL := ""
	if len(checkin.Media.Items) > 0 {
		photoURL = checkin.Media.Items[0].Photo.PhotoImgOg
	}

	metadata := &storage.CheckinMetadata{
		ID:             strconv.FormatUint(checkin.CheckinID, 10),
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
		Date:           createdAt,
		Style:          checkin.Beer.BeerStyle,
		ABV:            fmt.Sprintf("%.2f", checkin.Beer.BeerABV),
	}

	return downloader.DownloadAndSave(ctx, store, photoURL, cfg.PlaceholderPhotoPath, metadata)
}
