package sandbox

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	base, token string
	control     *http.Client
	transfer    *http.Client
}

// Production control traffic uses HTTPS and a dedicated runner token. Loopback
// HTTP is useful with a local SSH tunnel and never permits remote cleartext.
func NewClient(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") ||
		(u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()))) ||
		len(token) < 32 {
		return nil, errors.New("runner requires HTTPS (or loopback tunnel) and a dedicated token of at least 32 bytes")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		MaxIdleConnsPerHost: 4, ResponseHeaderTimeout: 90 * time.Second}
	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: strings.TrimRight(endpoint, "/"), token: token,
		control:  &http.Client{Timeout: 20 * time.Second, Transport: transport, CheckRedirect: noRedirect},
		transfer: &http.Client{Timeout: 2 * time.Minute, Transport: transport, CheckRedirect: noRedirect}}, nil
}

func (c *Client) request(ctx context.Context, method, path string, value any, transfer bool) (*http.Response, error) {
	var body io.Reader
	if value != nil {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	client := c.control
	if transfer {
		client = c.transfer
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		switch res.StatusCode {
		case 404:
			return nil, ErrMissing
		case 409:
			return nil, ErrConflict
		default:
			return nil, ErrUnavailable
		}
	}
	return res, nil
}

func (c *Client) json(ctx context.Context, method, path string, input, output any, transfer bool) error {
	res, err := c.request(ctx, method, path, input, transfer)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(output)
}

func (c *Client) Capability(ctx context.Context) (out Capability, err error) {
	err = c.json(ctx, "GET", "/v1/capability", nil, &out, false)
	if err == nil && (!out.Ready || !digest.MatchString(out.Image)) {
		err = ErrUnavailable
	}
	return
}

func (c *Client) Submit(ctx context.Context, request Request) (out Status, err error) {
	err = c.json(ctx, "PUT", "/v1/executions/"+request.ID, request, &out, true)
	return
}

func (c *Client) Get(ctx context.Context, id string) (out Status, err error) {
	if !safeID.MatchString(id) {
		return out, ErrMissing
	}
	err = c.json(ctx, "GET", "/v1/executions/"+id, nil, &out, false)
	return
}

func (c *Client) Cancel(ctx context.Context, id string) (out Status, err error) {
	if !safeID.MatchString(id) {
		return out, ErrMissing
	}
	err = c.json(ctx, "POST", "/v1/executions/"+id+"/cancel", nil, &out, false)
	return
}

func (c *Client) Artifact(ctx context.Context, id, artifact string) (io.ReadCloser, error) {
	if !safeID.MatchString(id) || !safeID.MatchString(artifact) {
		return nil, ErrMissing
	}
	res, err := c.request(ctx, "GET", "/v1/executions/"+id+"/artifacts/"+artifact, nil, true)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

func (c *Client) Forget(ctx context.Context, id string) error {
	if !safeID.MatchString(id) {
		return ErrMissing
	}
	var out Status
	return c.json(ctx, "DELETE", "/v1/executions/"+id, nil, &out, false)
}
