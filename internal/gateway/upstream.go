package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"ruralhealth/internal/config"
)

// Upstream represents one testing agency endpoint that the gateway can route to.
type Upstream struct {
	ID       string
	Name     string
	BaseURL  string
	Timeout  time.Duration
	Priority int
	client   *http.Client
}

func NewUpstream(cfg config.UpstreamConfig) *Upstream {
	return &Upstream{
		ID:       cfg.ID,
		Name:     cfg.Name,
		BaseURL:  cfg.BaseURL,
		Timeout:  cfg.Timeout,
		Priority: cfg.Priority,
		client:   &http.Client{Timeout: cfg.Timeout},
	}
}

// Call sends a POST to the upstream with the given path and body.
// Returns the response body, status code, and any error.
func (u *Upstream) Call(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	url := u.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("create request to %s: %w", u.ID, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Upstream-ID", u.ID)

	resp, err := u.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("call upstream %s: %w", u.ID, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response from %s: %w", u.ID, err)
	}
	return resp.StatusCode, respBody, nil
}

func (u *Upstream) String() string {
	return fmt.Sprintf("Upstream{id=%s name=%s url=%s priority=%d}", u.ID, u.Name, u.BaseURL, u.Priority)
}
