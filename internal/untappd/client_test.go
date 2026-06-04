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
	// a non-empty page that also reports the rate limit as exhausted: the
	// check-ins must still be processed, then pagination must stop.
	body := `{"response":{"checkins":{"items":[{"checkin_id":1}]},` +
		`"pagination":{"since_url":"https://api.untappd.com/v4/user/checkins?min_id=5"}}}`

	var requests int
	mockClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"X-RateLimit-Remaining": []string{"0"},
				},
				Body: io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	cfg := &config.Config{UntappdAccessToken: "test-token"}
	client := newTestClient(cfg, mockClient)

	var processorCalls int
	err := client.FetchCheckins(
		context.Background(),
		1,
		func(ctx context.Context, checkins []Checkin) error {
			processorCalls++
			return nil
		},
	)

	if err != nil {
		t.Fatalf("FetchCheckins returned error: %v", err)
	}
	if processorCalls != 1 {
		t.Errorf("expected checkinProcessor to be called once, got %d", processorCalls)
	}
	if requests != 1 {
		t.Errorf("expected to stop after one request when rate limited, got %d", requests)
	}
}

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
