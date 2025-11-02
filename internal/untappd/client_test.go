package untappd

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/smallwat3r/untappd-recorder/internal/config"
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
					"X-RateLimit-Remaining": []string{"0"},
				},
				Body: io.NopCloser(strings.NewReader(`{"response":{"checkins":{"items":[]}}}`)),
			}, nil
		}),
	}

	cfg := &config.Config{UntappdAccessToken: "test-token"}
	client := newTestClient(cfg, mockClient)

	processorCalled := false
	err := client.FetchCheckins(
		context.Background(),
		0,
		func(ctx context.Context, checkins []Checkin) error {
			processorCalled = true
			return nil
		},
	)

	if err != nil {
		t.Fatalf("FetchCheckins returned error: %v", err)
	}
	if processorCalled {
		t.Error("checkinProcessor should not be called when rate limit is reached")
	}
}

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
