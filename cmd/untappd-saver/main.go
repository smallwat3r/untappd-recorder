package main

import (
	"context"
	"fmt"
	"log"

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

	r2Client, err := storage.New(ctx, cfg)
	if err != nil {
		log.Fatalf("Error creating R2 client: %v", err)
	}
	latestCheckinID, err := r2Client.GetLatestCheckinID(ctx)
	if err != nil {
		log.Fatalf("Error getting latest checkin ID from R2: %v", err)
	}

	untappdClient := untappd.NewClient(cfg)
	err = untappdClient.FetchCheckins(ctx, latestCheckinID, func(ctx context.Context, checkins []untappd.Checkin) error {
		fmt.Printf("Processing %d checkins\n", len(checkins))
		for _, checkin := range checkins {
			log.Printf("Processing checkin %d", checkin.CheckinID)
			if err := r2Client.SaveCheckin(ctx, checkin); err != nil {
				log.Printf("Failed to save checkin %d: %v", checkin.CheckinID, err)
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Error fetching checkins: %v", err)
	}
}
