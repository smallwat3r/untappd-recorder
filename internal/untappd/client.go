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
	FetchCheckins(
		ctx context.Context,
		sinceID int,
		checkinProcessor func(context.Context, []Checkin) error,
	) error
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

func extractCheckins(r *UntappdResponse) ([]Checkin, error) {
	// when passing min_id in the querystring, the API bypasses checkins and
	// return items directly, we check here which one we should use.
	switch {
	case r.Response.Checkins != nil:
		return r.Response.Checkins.Items, nil
	case r.Response.Items != nil:
		return *r.Response.Items, nil
	default:
		return nil, fmt.Errorf("no checkins found in response")
	}
}

func (c *Client) handleResponse(
	ctx context.Context,
	resp *http.Response,
	checkinProcessor func(context.Context, []Checkin) error,
) (int, bool, error) {
	if resp.StatusCode != http.StatusOK {
		return 0, true, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	if resp.Header.Get("X-Ratelimit-Remaining") == "0" {
		fmt.Println("untappd API rate limit reached. Stopping for now.")
		return 0, true, nil
	}

	var untappdResp UntappdResponse
	if err := json.NewDecoder(resp.Body).Decode(&untappdResp); err != nil {
		return 0, true, fmt.Errorf("failed to decode response: %w", err)
	}

	checkins, err := extractCheckins(&untappdResp)
	if err != nil {
		return 0, true, err
	}

	if len(checkins) == 0 {
		return 0, true, nil
	}

	if err := checkinProcessor(ctx, checkins); err != nil {
		return 0, true, fmt.Errorf("failed to process checkins: %w", err)
	}

	sinceURL := untappdResp.Response.Pagination.SinceURL
	if sinceURL == "" {
		return 0, true, nil
	}

	nextMinID, err := parseMinID(sinceURL)
	if err != nil {
		return 0, true, fmt.Errorf(
			"failed to parse min_id from since_url %q: %w",
			sinceURL,
			err,
		)
	}

	return nextMinID, false, nil
}

func parseMinID(rawURL string) (int, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, fmt.Errorf("failed to parse URL %q: %w", rawURL, err)
	}

	minIDStr := u.Query().Get("min_id")
	if minIDStr == "" {
		return 0, fmt.Errorf("min_id not found in %q", rawURL)
	}

	v, err := strconv.ParseInt(minIDStr, 10, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to parse min_id %q: %w", minIDStr, err)
	}

	return int(v), nil
}

func (c *Client) buildRequest(
	ctx context.Context,
	endpoint string,
	minID int,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("access_token", c.cfg.UntappdAccessToken)
	if minID != 0 {
		q.Add("min_id", strconv.Itoa(minID))
	} else {
		// if sinceID is 0, it means we are starting from scratch, so we only
		// want to fetch the first checkin and stop.
		q.Add("limit", "1")
	}

	req.URL.RawQuery = q.Encode()
	return req, nil
}

func (c *Client) FetchCheckins(
	ctx context.Context,
	sinceID int,
	checkinProcessor func(context.Context, []Checkin) error,
) error {
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

		newMinID, shouldBreak, err := c.handleResponse(ctx, resp, checkinProcessor)
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
