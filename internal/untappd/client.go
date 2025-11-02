package untappd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/smallwat3r/untappd-recorder/internal/config"
)

type Client struct {
	cfg    *config.Config
	client *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) FetchCheckins(ctx context.Context, sinceID int, checkinProcessor func(context.Context, []Checkin) error) error {
	endpoint := "https://api.untappd.com/v4/user/checkins"
	maxID := 0

	for {
		req, err := c.buildRequest(ctx, endpoint, maxID, sinceID)
		if err != nil {
			return fmt.Errorf("failed to build request: %w", err)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		newMaxID, shouldBreak, err := c.handleResponse(ctx, resp, checkinProcessor)
		if err != nil {
			return fmt.Errorf("failed to handle response: %w", err)
		}

		if shouldBreak {
			break
		}
		maxID = newMaxID
	}
	return nil
}

func (c *Client) handleResponse(ctx context.Context, resp *http.Response, checkinProcessor func(context.Context, []Checkin) error) (int, bool, error) {
	if resp.StatusCode != http.StatusOK {
		return 0, true, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	if resp.Header.Get("X-Ratelimit-Remaining") == "0" {
		fmt.Println("Untappd API rate limit reached. Stopping for now.")
		return 0, true, nil
	}

	var untappdResp UntappdResponse
	if err := json.NewDecoder(resp.Body).Decode(&untappdResp); err != nil {
		return 0, true, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(untappdResp.Response.Checkins.Items) == 0 {
		return 0, true, nil
	}

	if err := checkinProcessor(ctx, untappdResp.Response.Checkins.Items); err != nil {
		return 0, true, fmt.Errorf("failed to process checkins: %w", err)
	}

	if untappdResp.Response.Pagination.MaxID == 0 {
		return 0, true, nil
	}

	return untappdResp.Response.Pagination.MaxID, false, nil
}

func (c *Client) buildRequest(ctx context.Context, endpoint string, maxID, sinceID int) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("access_token", c.cfg.UntappdAccessToken)
	if maxID != 0 {
		q.Add("max_id", fmt.Sprintf("%d", maxID))
	} else if sinceID != 0 {
		q.Add("min_id", fmt.Sprintf("%d", sinceID))
	} else {
		// if sinceID is 0, it means we are starting from scratch, so we only
		// want to fetch the first checkin and stop.
		q.Add("limit", "1")
	}
	req.URL.RawQuery = q.Encode()

	return req, nil
}
