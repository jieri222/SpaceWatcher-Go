package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"

	"github.com/jieri222/SpaceWatcher-Go/internal/client"
	"github.com/jieri222/SpaceWatcher-Go/internal/logger"
)

const (
	BearerToken = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
)

type TwitterSession struct {
	client          *client.Client
	guestToken      string
	queryID         string
	featureSwitches []string
}

// NewTwitterSession initializes a Session with Cookie management
func NewTwitterSession() *TwitterSession {
	jar, err := cookiejar.New(nil)
	if err != nil {
		// cookiejar.New(nil) should never fail, but handle defensively
		logger.Error("failed to create cookie jar", "error", err)
		panic("cookiejar.New: " + err.Error())
	}
	client := client.NewClient()
	client.HTTPClient.Jar = jar
	return &TwitterSession{
		client: client,
	}
}

func (s *TwitterSession) RefreshGuestToken() error {
	ctx := context.Background()

	// get cookies from x.com
	respCookies, err := s.client.Get(ctx, "https://x.com/", nil)
	if err != nil {
		return fmt.Errorf("fetch x.com for cookies: %w", err)
	}
	defer func() { _ = respCookies.Body.Close() }()

	// exchange guest token
	respToken, err := s.client.Post(ctx, "https://api.twitter.com/1.1/guest/activate.json",
		http.Header{
			"Authorization": {"Bearer " + BearerToken},
		})
	if err != nil {
		return fmt.Errorf("activate guest token: %w", err)
	}
	defer func() { _ = respToken.Body.Close() }()

	var res struct {
		GuestToken string `json:"guest_token"`
	}
	if err := json.NewDecoder(respToken.Body).Decode(&res); err != nil {
		return fmt.Errorf("decode guest token response: %w", err)
	}

	s.guestToken = res.GuestToken
	return nil
}

// GetGuestToken gets current guest token
func (s *TwitterSession) GetGuestToken() string {
	return s.guestToken
}

// SetQueryID sets GraphQL Query ID
func (s *TwitterSession) SetQueryID(id string) {
	s.queryID = id
}

// GetQueryID gets GraphQL Query ID
func (s *TwitterSession) GetQueryID() string {
	return s.queryID
}

// GetFeatureSwitches gets GraphQL Feature Switches
func (s *TwitterSession) GetFeatureSwitches() []string {
	return s.featureSwitches
}

// GetClient gets HTTP client
func (s *TwitterSession) GetClient() *client.Client {
	return s.client
}
