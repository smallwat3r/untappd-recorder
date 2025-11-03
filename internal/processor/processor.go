package processor

import (
	"context"

	"golang.org/x/sync/errgroup"
)

func Process[T any](
	ctx context.Context,
	items []T,
	numWorkers int,
	processFn func(ctx context.Context, item T),
) {
	g, ctx := errgroup.WithContext(ctx)
	if numWorkers > 0 {
		g.SetLimit(numWorkers)
	}

	for _, item := range items {
		item := item // capture
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			processFn(ctx, item)
			return nil
		})
	}

	_ = g.Wait()
}
