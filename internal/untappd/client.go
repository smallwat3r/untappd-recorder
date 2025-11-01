package untappd

import (
	"encoding/json"
	"fmt"
	"log"
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

func (c *Client) FetchCheckins(sinceID int, checkinProcessor func([]Checkin)) {
	endpoint := "https://api.untappd.com/v4/user/checkins"
	maxID := 0

	for {
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			log.Fatalf("Failed to create request: %v", err)
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
			log.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Fatalf("API request failed with status: %s", resp.Status)
		}

		var untappdResp UntappdResponse
		if err := json.NewDecoder(resp.Body).Decode(&untappdResp); err != nil {
			log.Fatalf("Failed to decode response: %v", err)
		}

		if len(untappdResp.Response.Checkins.Items) == 0 {
			break
		}

		checkinProcessor(untappdResp.Response.Checkins.Items)

		if untappdResp.Response.Pagination.MaxID == 0 {
			break
		}
		maxID = untappdResp.Response.Pagination.MaxID
	}
}
