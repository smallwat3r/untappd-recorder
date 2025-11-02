package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/photo"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
	"github.com/smallwat3r/untappd-recorder/internal/untappd"
)

func main() {
	if err := run(context.Background(), nil, nil); err != nil {
		log.Fatal(err)
	}
	fmt.Println("record completed successfully.")
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

	proc := newCheckinProcessor(store, cfg, downloader)
	return untappdClient.FetchCheckins(ctx, latestCheckinID, proc)
}

func newCheckinProcessor(
	store storage.Storage,
	cfg *config.Config,
	downloader photo.Downloader,
) func(context.Context, []untappd.Checkin) error {
	var once sync.Once

	return func(ctx context.Context, checkins []untappd.Checkin) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if len(checkins) == 0 {
			log.Printf("no checkins to process\n")
			return nil
		}

		log.Printf("processing %d checkins\n", len(checkins))
		processCheckins(ctx, store, cfg, checkins, downloader)

		// first element should be newest, update once per FetchCheckins cycle
		once.Do(func() {
			if err := store.UpdateLatestCheckinID(ctx, checkins[0]); err != nil {
				log.Printf("failed to update latest checkin ID: %v\n", err)
			}
		})
		return nil
	}
}

func processCheckins(
	ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	checkins []untappd.Checkin,
	downloader photo.Downloader,
) {
	g, ctx := errgroup.WithContext(ctx)
	workers := cfg.NumWorkers
	if workers <= 0 {
		workers = 10
	}
	g.SetLimit(workers)

	for _, c := range checkins {
		c := c // capture
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			log.Printf("processing checkin %d", c.CheckinID)
			if err := saveCheckin(ctx, store, cfg, c, downloader); err != nil {
				log.Printf("failed to save checkin %d: %v", c.CheckinID, err)
			}
			return nil
		})
	}

	_ = g.Wait()
}

func saveCheckin(
	ctx context.Context,
	store storage.Storage,
	cfg *config.Config,
	checkin untappd.Checkin,
	downloader photo.Downloader,
) error {
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
