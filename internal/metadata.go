package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// AudioSpaceById 取得 Space 的 metadata
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

	// 確保有 guest token
	if s.guestToken == "" {
		if err := s.RefreshGuestToken(); err != nil {
			return nil, fmt.Errorf("failed to refresh guest token: %w", err)
		}
	}

	// 組裝 URL
	apiURL, _ := url.Parse("https://api.x.com/graphql/" + s.queryID + "/AudioSpaceById")
	q := apiURL.Query()
	q.Set("variables", variables)
	q.Set("features", features)
	apiURL.RawQuery = q.Encode()

	// 發送請求
	resp, err := s.client.Get(ctx, apiURL.String(), http.Header{
		"Authorization": {"Bearer " + BearerToken},
		"x-guest-token": {s.guestToken},
		"Content-Type":  {"application/json"},
	})
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return parseAudioSpaceByIdResponse(body)
}

// AudioSpaceByIdResponse GraphQL 回應結構
type AudioSpaceByIdResponse struct {
	Data struct {
		AudioSpace struct {
			Metadata     SpaceMetadata    `json:"metadata"`
			Participants SpaceParticipant `json:"participants"`
		} `json:"audioSpace"`
	} `json:"data"`
}

// SpaceMetadata Space 的 metadata
type SpaceMetadata struct {
	RestID         string `json:"rest_id"`
	State          string `json:"state"`
	Title          string `json:"title"`
	MediaKey       string `json:"media_key"`
	StartedAt      int64  `json:"started_at"`
	CreatorResults struct {
		Result struct {
			Core struct {
				Name       string `json:"name"`
				ScreenName string `json:"screen_name"`
			} `json:"core"`
		} `json:"result"`
	} `json:"creator_results"`
}

// SpaceParticipant Space 的參與者
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
