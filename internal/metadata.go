package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jieri222/SpaceWatcher-Go/internal/logger"
)

// AudioSpaceById retrieves Space metadata given a Space ID
func (s *TwitterSession) AudioSpaceById(spaceID string) (*AudioSpaceByIdResponse, error) {
	ctx := context.Background()

	variables := fmt.Sprintf(`{
		"id": "%s",
		"isMetatagsQuery": false,
		"withReplays": true,
		"withListeners": true
	}`, spaceID)

	featureSwitches := s.GetFeatureSwitches()
	features := "{" + strings.Join(featureSwitches, ",") + "}"

	// Build URL
	apiURL, err := url.Parse("https://api.x.com/graphql/" + s.queryID + "/AudioSpaceById")
	if err != nil {
		return nil, fmt.Errorf("parse AudioSpaceById URL: %w", err)
	}
	q := apiURL.Query()
	q.Set("variables", variables)
	q.Set("features", features)
	apiURL.RawQuery = q.Encode()

	// Send request
	body, err := doAudioSpaceByIdRequest(ctx, s, apiURL.String(), 0)
	if err != nil {
		return nil, err
	}

	return parseAudioSpaceByIdResponse(body)
}

func doAudioSpaceByIdRequest(ctx context.Context, session *TwitterSession, url string, retryCount int) ([]byte, error) {
	// Ensure guest token is present
	if session.guestToken == "" {
		if err := session.RefreshGuestToken(); err != nil {
			return nil, fmt.Errorf("failed to refresh guest token: %w", err)
		}
	}

	resp, err := session.client.Get(ctx, url, http.Header{
		"Authorization": {"Bearer " + BearerToken},
		"x-guest-token": {session.guestToken},
		"Content-Type":  {"application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("request AudioSpaceById: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read AudioSpaceById response: %w", err)
	}

	switch resp.StatusCode {
	case 200:
		return body, nil
	case 403: // Upon 403 error, attempt to refresh token and retry
		if retryCount >= 3 {
			return nil, fmt.Errorf("API error 403 after retry: %s", string(body))
		}
		logger.Warn("Guest token might be expired, attempting to refresh", "statusCode", resp.StatusCode)
		if err := session.RefreshGuestToken(); err != nil {
			return nil, fmt.Errorf("failed to refresh guest token after 403: %w", err)
		}
		logger.Debug("Guest token refreshed", "token", session.guestToken)
		return doAudioSpaceByIdRequest(ctx, session, url, retryCount+1)
	default:
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
}

// AudioSpaceByIdResponse represents the structure of the GraphQL response
type AudioSpaceByIdResponse struct {
	Data struct {
		AudioSpace struct {
			Metadata     SpaceMetadata    `json:"metadata"`
			Participants SpaceParticipant `json:"participants"`
		} `json:"audioSpace"`
	} `json:"data"`
}

// SpaceMetadata defines metadata associated with a Space
type SpaceMetadata struct {
	RestID                    string `json:"rest_id"`
	State                     string `json:"state"`
	Title                     string `json:"title"`
	MediaKey                  string `json:"media_key"`
	StartedAt                 int64  `json:"started_at"`
	IsSpaceAvailableforReplay bool   `json:"is_space_available_for_replay"`
	CreatorResults            struct {
		Result struct {
			Core struct {
				Name       string `json:"name"`
				ScreenName string `json:"screen_name"`
			} `json:"core"`
		} `json:"result"`
	} `json:"creator_results"`
}

// SpaceParticipant outlines participants within the Space
type SpaceParticipant struct {
	Admins []struct {
		TwitterScreenName string `json:"twitter_screen_name"`
		DisplayName       string `json:"display_name"`
	} `json:"admins"`
	Speakers []struct {
		TwitterScreenName string `json:"twitter_screen_name"`
		DisplayName       string `json:"display_name"`
	} `json:"speakers"`
}

func parseAudioSpaceByIdResponse(body []byte) (*AudioSpaceByIdResponse, error) {
	var resp AudioSpaceByIdResponse
	err := json.Unmarshal(body, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w\nBody: %s", err, string(body))
	}
	return &resp, nil
}
