// Command blurfaces reprocesses every photo already stored in the bucket,
// blurring detected faces and replacing the objects in place. Intended to be
// run locally as a one-off; new checkins are blurred at ingestion when
// BLUR_FACES is set.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/smallwat3r/untappd-recorder/internal/config"
	"github.com/smallwat3r/untappd-recorder/internal/faceblur"
	"github.com/smallwat3r/untappd-recorder/internal/photo"
	"github.com/smallwat3r/untappd-recorder/internal/processor"
	"github.com/smallwat3r/untappd-recorder/internal/storage"
)

type store interface {
	ListJPGKeys(ctx context.Context) ([]string, error)
	DownloadWithMetadata(ctx context.Context, key string) ([]byte, map[string]string, error)
	Replace(ctx context.Context, key string, b []byte, md map[string]string, contentType string) error
}

func main() {
	dryRun := flag.Bool("dry-run", false, "report faces without modifying the bucket")
	minQuality := flag.Float64(
		"min-quality",
		faceblur.DefaultMinQuality,
		"detection confidence cutoff, raise it if non-faces get blurred, "+
			"lower it if real faces are missed (use -dry-run to see scores)",
	)
	flag.Parse()

	detect := func(b []byte) ([]faceblur.Face, error) {
		return faceblur.Detect(b, float32(*minQuality))
	}

	if err := run(
		context.Background(), *dryRun, nil, detect, faceblur.BlurFaces, photo.ToWEBP,
	); err != nil {
		log.Fatalf("blurfaces failed: %v", err)
	}
}

func run(
	ctx context.Context,
	dryRun bool,
	st store,
	detect func([]byte) ([]faceblur.Face, error),
	blur func([]byte, []faceblur.Face) ([]byte, error),
	toWEBP func([]byte) ([]byte, error),
) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}

	if st == nil {
		c, err := storage.NewClient(ctx, cfg)
		if err != nil {
			return fmt.Errorf("error creating storage client: %w", err)
		}
		st = c
	}

	keys, err := st.ListJPGKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to list photos: %w", err)
	}
	log.Printf("Found %d photos to scan (dry-run: %v)", len(keys), dryRun)

	var blurred, faces, failed atomic.Int64

	processor.Process(ctx, keys, cfg.NumWorkers, func(ctx context.Context, key string) {
		n, err := processKey(ctx, dryRun, st, detect, blur, toWEBP, key)
		if err != nil {
			failed.Add(1)
			log.Printf("failed to process %q: %v", key, err)
			return
		}
		if n > 0 {
			blurred.Add(1)
			faces.Add(int64(n))
		}
	})

	log.Printf(
		"Done: %d photos scanned, %d with faces (%d faces), %d failed",
		len(keys), blurred.Load(), faces.Load(), failed.Load(),
	)
	if failed.Load() > 0 {
		return fmt.Errorf("%d photos failed, re-run to retry them", failed.Load())
	}
	return nil
}

func processKey(
	ctx context.Context,
	dryRun bool,
	st store,
	detect func([]byte) ([]faceblur.Face, error),
	blur func([]byte, []faceblur.Face) ([]byte, error),
	toWEBP func([]byte) ([]byte, error),
	key string,
) (int, error) {
	b, md, err := st.DownloadWithMetadata(ctx, key)
	if err != nil {
		return 0, fmt.Errorf("download: %w", err)
	}

	faces, err := detect(b)
	if err != nil {
		return 0, fmt.Errorf("detect: %w", err)
	}
	if len(faces) == 0 {
		return 0, nil
	}

	if dryRun {
		for _, f := range faces {
			log.Printf(
				"would blur face in %q: confidence=%.1f size=%dpx at (%d,%d)",
				key, f.Q, f.Rect.Dx(), f.Rect.Min.X, f.Rect.Min.Y,
			)
		}
		return len(faces), nil
	}

	out, err := blur(b, faces)
	if err != nil {
		return 0, fmt.Errorf("blur: %w", err)
	}

	// write the WebP sibling before overwriting the JPG: the unblurred JPG
	// is what detection keys off, so a failure between the two writes is
	// repaired by a re-run. In the reverse order a blurred JPG next to a
	// stale unblurred WebP would never be detected again.
	if webpKey := storage.WEBPSiblingKey(key); webpKey != "" {
		webp, err := toWEBP(out)
		if err != nil {
			return 0, fmt.Errorf("webp conversion: %w", err)
		}
		if err := st.Replace(ctx, webpKey, webp, md, "image/webp"); err != nil {
			return 0, fmt.Errorf("replace webp: %w", err)
		}
	}

	if err := st.Replace(ctx, key, out, md, "image/jpeg"); err != nil {
		return 0, fmt.Errorf("replace jpg: %w", err)
	}

	log.Printf("blurred %d face(s) in %q", len(faces), key)
	return len(faces), nil
}
