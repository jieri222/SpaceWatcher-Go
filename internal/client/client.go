package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Client struct holds the HTTP client.
type Client struct {
	HTTPClient     *http.Client
	DefaultHeaders http.Header
}

// NewClient creates a new Client instance with optimized connection pool.
func NewClient() *Client {
	// Optimized Transport configuration
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20, // Increase connection pool per host
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		HTTPClient: &http.Client{
			Timeout:   60 * time.Second, // Increase to 60s for large files
			Transport: transport,
		},
		DefaultHeaders: http.Header{
			"User-Agent": {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"},
		},
	}
}

func (c *Client) Get(ctx context.Context, url string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	if headers != nil {
		req.Header = headers.Clone()
	}
	// set default headers
	for k, vals := range c.DefaultHeaders {
		if req.Header.Get(k) == "" {
			for _, v := range vals {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending GET request: %w", err)
	}

	return resp, nil
}

func (c *Client) Post(ctx context.Context, url string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	if headers != nil {
		req.Header = headers.Clone()
	}
	// set default headers
	for k, vals := range c.DefaultHeaders {
		if req.Header.Get(k) == "" {
			for _, v := range vals {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending POST request: %w", err)
	}

	return resp, nil
}
