package untappd

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/smallwat3r/untappd-saver/internal/config"
)

func newTestClient(cfg *config.Config, client *http.Client) *Client {
	return &Client{
		cfg:    cfg,
		client: client,
	}
}

func TestFetchCheckins_RateLimit(t *testing.T) {
	mockClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"X-Ratelimit-Remaining": []string{"0"},
				},
				Body: io.NopCloser(strings.NewReader(`{"response":{"checkins":{"items":[]}}}`)),
			}, nil
		}),
	}

	cfg := &config.Config{UntappdAccessToken: "test-token"}
	client := newTestClient(cfg, mockClient)

	err := client.FetchCheckins(context.Background(), 0, func(ctx context.Context, checkins []Checkin) error {
		t.Error("checkinProcessor should not be called when rate limit is reached")
		return nil
	})

	if err != nil {
		t.Errorf("FetchCheckins returned an error: %v", err)
	}
}

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
