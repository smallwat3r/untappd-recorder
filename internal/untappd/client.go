package untappd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/smallwat3r/untappd-recorder/internal/config"
)

type UntappdClient interface {
	FetchCheckins(ctx context.Context, sinceID int, checkinProcessor func(context.Context, []Checkin) error) error
}

type Client struct {
	cfg    *config.Config
	client *http.Client
}

func NewClient(cfg *config.Config) UntappdClient {
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) handleResponse(ctx context.Context, resp *http.Response, checkinProcessor func(context.Context, []Checkin) error, minIDInQuery bool) (int, bool, error) {
	if resp.StatusCode != http.StatusOK {
		return 0, true, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	if resp.Header.Get("X-Ratelimit-Remaining") == "0" {
		fmt.Println("Untappd API rate limit reached. Stopping for now.")
		return 0, true, nil
	}

	var checkins []Checkin
	var paginationSinceURL string

	// a bit ugly, but for some reason the API does not return the same data shape if `min_id`
	// is passed to the querystring.
	if minIDInQuery {
		var untappdRespMinID UntappdResponseMinID
		if err := json.NewDecoder(resp.Body).Decode(&untappdRespMinID); err != nil {
			return 0, true, fmt.Errorf("failed to decode response with min_id: %w", err)
		}
		checkins = untappdRespMinID.Response.Items
		paginationSinceURL = untappdRespMinID.Response.Pagination.SinceURL
	} else {
		var untappdResp UntappdResponse
		if err := json.NewDecoder(resp.Body).Decode(&untappdResp); err != nil {
			return 0, true, fmt.Errorf("failed to decode response: %w", err)
		}
		checkins = untappdResp.Response.Checkins.Items
		paginationSinceURL = untappdResp.Response.Pagination.SinceURL
	}

	if len(checkins) == 0 {
		return 0, true, nil
	}

	if err := checkinProcessor(ctx, checkins); err != nil {
		return 0, true, fmt.Errorf("failed to process checkins: %w", err)
	}

	if paginationSinceURL == "" {
		return 0, true, nil
	}

	nextMinID, err := parseMinID(paginationSinceURL)
	if err != nil {
		return 0, true, fmt.Errorf("failed to parse min_id from since_url: %w", err)
	}

	return nextMinID, false, nil
}

func parseMinID(sinceURL string) (int, error) {
	u, err := url.Parse(sinceURL)
	if err != nil {
		return 0, err
	}

	minIDStr := u.Query().Get("min_id")
	if minIDStr == "" {
		return 0, fmt.Errorf("min_id not found in since_url")
	}

	minID, err := strconv.Atoi(minIDStr)
	if err != nil {
		return 0, err
	}

	return minID, nil
}

func (c *Client) buildRequest(ctx context.Context, endpoint string, minID int) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("access_token", c.cfg.UntappdAccessToken)
	if minID != 0 {
		q.Add("min_id", fmt.Sprintf("%d", minID))
	} else {
		// if sinceID is 0, it means we are starting from scratch, so we only
		// want to fetch the first checkin and stop.
		q.Add("limit", "1")
	}

	req.URL.RawQuery = q.Encode()
	return req, nil
}

func (c *Client) FetchCheckins(ctx context.Context, sinceID int, checkinProcessor func(context.Context, []Checkin) error) error {
	endpoint := "https://api.untappd.com/v4/user/checkins"
	minID := sinceID

	for {
		req, err := c.buildRequest(ctx, endpoint, minID)
		if err != nil {
			return err
		}

		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}

		defer resp.Body.Close()

		newMinID, shouldBreak, err := c.handleResponse(ctx, resp, checkinProcessor, minID != 0)
		if err != nil {
			return err
		}
		if shouldBreak {
			break
		}

		minID = newMinID
	}

	return nil
}
