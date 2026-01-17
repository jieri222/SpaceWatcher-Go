package internal

import (
	"context"
	"fmt"
	"io"
	"regexp"
)

const baseUrl = "https://x.com"

// 備用 QueryID (會定期更新，如果失敗可手動更新)
// 可從瀏覽器 DevTools Network 找到 AudioSpaceById 請求
const FallbackQueryID = "_TgkQtc04XURgCocb1y9CA"

// Bootstrap 從 Space URL 取得 QueryID 並設定到 session
func (s *TwitterSession) Bootstrap(spaceID string) error {
	jsHash, err := s.findJSHash(spaceID)
	if err != nil {
		// 使用 fallback
		Warn("無法取得 JS hash，使用備用 QueryID", "error", err, "fallbackQueryID", FallbackQueryID)
		s.queryID = FallbackQueryID
		return nil
	}

	queryID, featureSwitches, err := s.findQueryID(jsHash)
	if err != nil {
		// 使用 fallback
		Warn("無法取得 QueryID，使用備用 QueryID", "error", err, "fallbackQueryID", FallbackQueryID)
		s.queryID = FallbackQueryID
		return nil
	}

	Debug("Bootstrap 完成", "queryID", queryID, "featureSwitches", featureSwitches)
	s.queryID = queryID
	s.featureSwitches = featureSwitches
	return nil
}

func (s *TwitterSession) findJSHash(spaceID string) (string, error) {
	ctx := context.Background()
	spaceURL := fmt.Sprintf("%s/i/spaces/%s/peek", baseUrl, spaceID)

	resp, err := s.client.Get(ctx, spaceURL, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	pattern := `"modules\.audio":\s*"([a-zA-Z0-9]+)"`

	re := regexp.MustCompile(pattern)
	match := re.FindSubmatch(body)
	if len(match) > 1 {
		return string(match[1]), nil
	}

	return "", fmt.Errorf("could not find JS hash in HTML (body length: %d)", len(body))
}

// QueryInfo 存儲從 JS 提取的 GraphQL 查詢資訊
type QueryInfo struct {
	QueryID         string
	FeatureSwitches []string
}

func (s *TwitterSession) findQueryID(jsHash string) (string, []string, error) {
	ctx := context.Background()
	url := fmt.Sprintf("https://abs.twimg.com/responsive-web/client-web/modules.audio.%sa.js", jsHash)

	resp, err := s.client.Get(ctx, url, nil)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	// 提取 queryId 和整個 metadata 區塊
	// 格式: queryId:"xxx",operationName:"AudioSpaceById",operationType:"query",metadata:{featureSwitches:[...]}
	pattern := `queryId:"([a-zA-Z0-9_-]+)",operationName:"AudioSpaceById",operationType:"query",metadata:\{featureSwitches:\[([^\]]*)\]`

	re := regexp.MustCompile(pattern)
	match := re.FindSubmatch(body)
	if len(match) < 2 {
		return "", nil, fmt.Errorf("could not find queryID in JS files")
	}

	queryID := string(match[1])

	// 提取 featureSwitches 陣列內容
	var featureSwitches []string
	if len(match) > 2 && len(match[2]) > 0 {
		featurePattern := `"([^"]+)"`
		featureRe := regexp.MustCompile(featurePattern)
		featureMatches := featureRe.FindAllSubmatch(match[2], -1)
		for _, fm := range featureMatches {
			if len(fm) > 1 {
				featureSwitches = append(featureSwitches, fmt.Sprintf(`"%s":false`, string(fm[1])))
			}
		}
	}

	return queryID, featureSwitches, nil
}
