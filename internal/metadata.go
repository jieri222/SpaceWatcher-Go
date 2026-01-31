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

	// 組裝 URL
	apiURL, _ := url.Parse("https://api.x.com/graphql/" + s.queryID + "/AudioSpaceById")
	q := apiURL.Query()
	q.Set("variables", variables)
	q.Set("features", features)
	apiURL.RawQuery = q.Encode()

	// 發送請求
	body, err := doAudioSapcebyIdRequest(ctx, s, apiURL.String())
	if err != nil {
		return nil, err
	}

	return parseAudioSpaceByIdResponse(body)
}

func doAudioSapcebyIdRequest(ctx context.Context, session *TwitterSession, url string) ([]byte, error) {
	// 確保有 guest token
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
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case 200:
		return body, nil
	case 403: // 如果是 403，嘗試刷新 token 並重試
		Warn("Guest token 可能已過期，嘗試刷新", "statusCode", resp.StatusCode)
		if err := session.RefreshGuestToken(); err != nil {
			return nil, fmt.Errorf("failed to refresh guest token after 403: %w", err)
		}
		Debug("Guest token 已刷新", "newToken", session.guestToken)
		return doAudioSapcebyIdRequest(ctx, session, url)
	default:
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
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
