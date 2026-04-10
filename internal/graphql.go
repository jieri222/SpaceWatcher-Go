package internal

import (
	"context"
	"fmt"
	"io"
	"regexp"

	"github.com/jieri222/SpaceWatcher-Go/internal/logger"
)

const baseUrl = "https://x.com"

// FallbackQueryID (updated periodically, can be updated manually if extraction fails)
// Can be found via browser DevTools: Network -> search for "AudioSpaceById" request
const FallbackQueryID = "_TgkQtc04XURgCocb1y9CA"

// DiscoverQueryID extracts the QueryID from x.com dynamically and sets it in the session
func (s *TwitterSession) DiscoverQueryID() error {
	jsHash, err := s.extractJSHashFromPage()
	if err != nil {
		// Use fallback
		logger.Warn("Failed to locate JS hash, using fallback QueryID", "error", err, "fallbackQueryID", FallbackQueryID)
		s.queryID = FallbackQueryID
		return nil
	}

	queryID, featureSwitches, err := s.parseQueryIDFromJS(jsHash)
	if err != nil {
		// Use fallback
		logger.Warn("Failed to parse QueryID, using fallback QueryID", "error", err, "fallbackQueryID", FallbackQueryID)
		s.queryID = FallbackQueryID
		return nil
	}

	logger.Debug("Got QueryID", "queryID", queryID, "featureSwitches", featureSwitches)
	s.queryID = queryID
	s.featureSwitches = featureSwitches
	return nil
}

func (s *TwitterSession) extractJSHashFromPage() (string, error) {
	ctx := context.Background()

	resp, err := s.client.Get(ctx, baseUrl, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Step 1: Find the chunk ID, e.g., 23441: "modules.audio"
	chunkIDPattern := regexp.MustCompile(`(\d+):\s*"modules\.audio"`)
	chunkMatch := chunkIDPattern.FindSubmatch(body)
	if len(chunkMatch) < 2 {
		return "", fmt.Errorf("could not find modules.audio chunk ID (body length: %d)", len(body))
	}
	chunkID := string(chunkMatch[1])

	// Step 2: Find the corresponding hash, e.g., 23441: "d85c73e"
	hashPattern := regexp.MustCompile(chunkID + `:\s*"([a-fA-F0-9]+)"`)
	hashMatch := hashPattern.FindSubmatch(body)
	if len(hashMatch) < 2 {
		return "", fmt.Errorf("could not find hash for chunk ID %s (body length: %d)", chunkID, len(body))
	}

	return string(hashMatch[1]), nil
}

// QueryInfo stores the GraphQL query information extracted from JS files
type QueryInfo struct {
	QueryID         string
	FeatureSwitches []string
}

func (s *TwitterSession) parseQueryIDFromJS(jsHash string) (string, []string, error) {
	ctx := context.Background()
	url := fmt.Sprintf("https://abs.twimg.com/responsive-web/client-web/modules.audio.%sa.js", jsHash)

	resp, err := s.client.Get(ctx, url, nil)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	// Extract queryId and the entire metadata block
	// Format: queryId:"xxx",operationName:"AudioSpaceById",operationType:"query",metadata:{featureSwitches:[...]}
	pattern := `queryId:"([a-zA-Z0-9_-]+)",operationName:"AudioSpaceById",operationType:"query",metadata:\{featureSwitches:\[([^\]]*)\]`

	re := regexp.MustCompile(pattern)
	match := re.FindSubmatch(body)
	if len(match) < 2 {
		return "", nil, fmt.Errorf("could not find queryID in JS files")
	}

	queryID := string(match[1])

	// Extract featureSwitches array content
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
