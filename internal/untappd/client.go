package untappd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/smallwat3r/untappd-saver/internal/config"
)

type Client struct {
	cfg    *config.Config
	client *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg:    cfg,
		client: &http.Client{},
	}
}

func (c *Client) FetchCheckins(ctx context.Context, sinceID int, checkinProcessor func(context.Context, []Checkin) error) error {
	endpoint := "https://api.untappd.com/v4/user/checkins"
	maxID := 0

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		q := req.URL.Query()
		q.Add("access_token", c.cfg.UntappdAccessToken)
		if maxID != 0 {
			q.Add("max_id", fmt.Sprintf("%d", maxID))
		} else if sinceID != 0 {
			q.Add("min_id", fmt.Sprintf("%d", sinceID))
		}
		req.URL.RawQuery = q.Encode()

		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("API request failed with status: %s", resp.Status)
		}

		if resp.Header.Get("X-Ratelimit-Remaining") == "0" {
			fmt.Println("Untappd API rate limit reached. Stopping for now.")
			break
		}

		var untappdResp UntappdResponse
		if err := json.NewDecoder(resp.Body).Decode(&untappdResp); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		if len(untappdResp.Response.Checkins.Items) == 0 {
			break
		}

		if err := checkinProcessor(ctx, untappdResp.Response.Checkins.Items); err != nil {
			return fmt.Errorf("failed to process checkins: %w", err)
		}

		if untappdResp.Response.Pagination.MaxID == 0 {
			break
		}
		maxID = untappdResp.Response.Pagination.MaxID
	}
	return nil
}
