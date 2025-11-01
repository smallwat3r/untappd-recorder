package main

import (
	"fmt"
	"log"

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

	untappdClient := untappd.NewClient(cfg)
	checkins := untappdClient.FetchCheckins()

		r2Client := storage.NewR2Client(cfg)

		for _, checkin := range checkins {

			r2Client.SaveCheckin(checkin)

		}

	}

	