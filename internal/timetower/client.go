package timetower

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client is the Enforcer-side Timetower API client.
type Client struct {
	baseURL string
	version string
	http    *http.Client
}

// NewClient returns a client for the Timetower server at baseURL.
func NewClient(baseURL, version string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		version: version,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// FetchSchedule returns the schedule and its revision.
func (c *Client) FetchSchedule(ctx context.Context) (*Schedule, int64, error) {
	var schedule Schedule
	revision, err := c.do(ctx, http.MethodGet, "/api/v1/schedule", nil, &schedule)
	if err != nil {
		return nil, 0, err
	}
	return &schedule, revision, nil
}

// SendStatus reports Enforcer status and returns the server response and the
// current schedule revision.
func (c *Client) SendStatus(ctx context.Context, request StatusRequest) (*StatusResponse, int64, error) {
	var response StatusResponse
	revision, err := c.do(ctx, http.MethodPut, "/api/v1/status", request, &response)
	if err != nil {
		return nil, 0, err
	}
	return &response, revision, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) (int64, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("timetower: encode request: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, fmt.Errorf("timetower: create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.version != "" {
		request.Header.Set("User-Agent", "timelord-enforcer/"+c.version)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return 0, fmt.Errorf("timetower: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		detail := strings.TrimSpace(string(data))
		if detail == "" {
			detail = http.StatusText(response.StatusCode)
		}
		return 0, fmt.Errorf("timetower: %s: %s", response.Status, detail)
	}

	revision, _ := strconv.ParseInt(response.Header.Get("X-Schedule-Revision"), 10, 64)
	if out != nil {
		if err := json.NewDecoder(response.Body).Decode(out); err != nil {
			return 0, fmt.Errorf("timetower: decode response: %w", err)
		}
	}
	return revision, nil
}
