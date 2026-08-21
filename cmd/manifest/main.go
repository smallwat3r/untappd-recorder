// Command manifest rebuilds the bucket's index.json from scratch by walking
// every WEBP object and reading its metadata. The manifest is derived data,
// the bucket stays the single source of truth, so this is the repair path
// whenever index.json is wrong or lost. Day-to-day the record and backfill
// commands keep the manifest up to date incrementally.
package main

import (
	"context"
	"log"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("error loading configuration: %v", err)
	}

	client, err := storage.NewClient(ctx, cfg)
	if err != nil {
		log.Fatalf("error creating storage client: %v", err)
	}

	log.Println("Rebuilding manifest from the bucket...")
	n, err := client.RebuildManifest(ctx, cfg.NumWorkers)
	if err != nil {
		log.Fatalf("manifest rebuild failed: %v", err)
	}
	log.Printf("Manifest rebuilt: %d records covering %d WEBP objects.", n, n)
}
