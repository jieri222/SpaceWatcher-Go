package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"spacewatcher/internal/client"
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

// NewTwitterSession 初始化一個帶有 Cookie 管理的 Session
func NewTwitterSession() *TwitterSession {
	jar, _ := cookiejar.New(nil)
	client := client.NewClient()
	client.HTTPClient.Jar = jar
	return &TwitterSession{
		client: client,
	}
}

func (s *TwitterSession) RefreshGuestToken() error {
	ctx := context.Background()

	// get cookies from x.com
	if _, err := s.client.Get(ctx, "https://x.com/", nil); err != nil {
		return err
	}

	// exchange guest token
	resp, err := s.client.Post(ctx, "https://api.twitter.com/1.1/guest/activate.json",
		http.Header{
			"Authorization": {"Bearer " + BearerToken},
		})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var res struct {
		GuestToken string `json:"guest_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	s.guestToken = res.GuestToken
	return nil
}

// GetGuestToken 取得當前 guest token
func (s *TwitterSession) GetGuestToken() string {
	return s.guestToken
}

// SetQueryID 設定 GraphQL Query ID
func (s *TwitterSession) SetQueryID(id string) {
	s.queryID = id
}

// GetQueryID 取得 GraphQL Query ID
func (s *TwitterSession) GetQueryID() string {
	return s.queryID
}

// GetFeatureSwitches 取得 GraphQL Feature Switches
func (s *TwitterSession) GetFeatureSwitches() []string {
	return s.featureSwitches
}

// GetClient 取得 HTTP client
func (s *TwitterSession) GetClient() *client.Client {
	return s.client
}
