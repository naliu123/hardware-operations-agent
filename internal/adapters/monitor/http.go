// Package monitor implements the read-only observation gateway contract.
package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"hwops/internal/domain"
)

type HTTP struct {
	endpoint string
	key      string
	mode     string
	client   *http.Client
	slots    chan struct{}
}

func New(endpoint, key, mode string, timeout time.Duration, concurrency int) (*HTTP, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "http" && u.Scheme != "https") || (mode != "LIVE" && mode != "REPLAY") ||
		timeout <= 0 || timeout > time.Minute || concurrency < 1 || concurrency > 32 {
		return nil, fmt.Errorf("invalid monitor endpoint, mode, timeout or concurrency")
	}
	return &HTTP{endpoint: endpoint, key: key, mode: mode, slots: make(chan struct{}, concurrency),
		client: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}}, nil
}

func (m *HTTP) Observe(ctx context.Context, q domain.ObservationQuery) (domain.ObservationData, error) {
	var data domain.ObservationData
	if q.Device.DataMode != m.mode {
		return data, fmt.Errorf("monitor data mode mismatch")
	}
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		return data, ctx.Err()
	}
	raw, err := json.Marshal(q)
	if err != nil {
		return data, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(raw))
	if err != nil {
		return data, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.key != "" {
		req.Header.Set("Authorization", "Bearer "+m.key)
	}
	res, err := m.client.Do(req)
	if err != nil {
		return data, fmt.Errorf("monitor unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return data, fmt.Errorf("monitor HTTP %d", res.StatusCode)
	}
	raw, err = io.ReadAll(io.LimitReader(res.Body, 1024*1024+1))
	if err != nil || len(raw) > 1024*1024 {
		return data, fmt.Errorf("monitor response unreadable")
	}
	err = json.Unmarshal(raw, &data)
	return data, err
}
