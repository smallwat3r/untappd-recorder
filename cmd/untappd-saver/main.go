package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/smallwat3r/untappd-saver/internal/config"
	"github.com/smallwat3r/untappd-saver/internal/storage"
	"github.com/smallwat3r/untappd-saver/internal/untappd"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	fmt.Println("Successfully loaded configuration.")

	r2Client := storage.New(cfg)
	latestCheckinID, err := r2Client.GetLatestCheckinID()
	if err != nil {
		log.Fatalf("Error getting latest checkin ID from R2: %v", err)
	}

	untappdClient := untappd.NewClient(cfg)
	checkins := untappdClient.FetchCheckins(latestCheckinID)

	var wg sync.WaitGroup
	checkinChan := make(chan untappd.Checkin, len(checkins))

	for i := 0; i < cfg.NumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for checkin := range checkinChan {
				log.Printf("Processing checkin %d", checkin.CheckinID)
				if err := r2Client.SaveCheckin(checkin); err != nil {
					log.Printf("Failed to save checkin %d: %v", checkin.CheckinID, err)
				}
			}
		}()
	}

	for _, checkin := range checkins {
		checkinChan <- checkin
	}
	close(checkinChan)

	wg.Wait()
}
