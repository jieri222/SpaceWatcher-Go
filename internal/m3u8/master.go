package m3u8

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/jieri222/SpaceWatcher-Go/internal/client"
	"github.com/jieri222/SpaceWatcher-Go/internal/logger"

	"github.com/Eyevinn/hls-m3u8/m3u8"
)

// GetMasterPlaylistURL converts a dynamic playlist URL to a master playlist URL
func GetMasterPlaylistURL(ctx context.Context, client *client.Client, mediaKey string) (string, error) {
	dynamicURL, err := GetSourceLocation(ctx, client, mediaKey)
	if err != nil {
		return "", err
	}
	baseUrl := strings.Split(dynamicURL, "/")
	baseUrl = baseUrl[:len(baseUrl)-1]
	return strings.Join(baseUrl, "/") + "/master_playlist.m3u8", nil
}

// ResolveMasterPlaylist parses the master playlist to get the media playlist URL nested inside.
// A master playlist lists different bandwidth variants; this function selects the first one.
func ResolveMasterPlaylist(ctx context.Context, client *client.Client, masterURL string) (string, error) {
	resp, err := client.Get(ctx, masterURL, nil)
	if err != nil {
		return "", fmt.Errorf("fetch master playlist: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read master playlist body: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("master playlist HTTP error %d: %s", resp.StatusCode, string(body))
	}

	playlist, _, err := m3u8.DecodeFrom(strings.NewReader(string(body)), false)
	if err != nil {
		return "", fmt.Errorf("decode master playlist: %w", err)
	}

	master, ok := playlist.(*m3u8.MasterPlaylist)
	if !ok {
		// If it's already a media playlist, return the masterURL directly
		return masterURL, nil
	}

	variants := master.Variants
	if len(variants) == 0 {
		return "", fmt.Errorf("master playlist has no variants")
	}

	// Obtain the URI of the first variant
	variantURI := variants[0].URI
	if strings.HasPrefix(variantURI, "http") {
		return variantURI, nil
	}

	// Relative path, construct with the base URL (handles overlapping paths)
	baseURL := masterURL[:strings.LastIndex(masterURL, "/")+1]
	mediaURL, err := resolveOverlappingPath(baseURL, variantURI)
	if err != nil {
		return "", fmt.Errorf("resolve variant path: %w", err)
	}

	// Change "transcode" to "non_transcode"
	mediaURL = strings.Replace(mediaURL, "transcode", "non_transcode", 1)

	// Remove JWT token piece: the exact part strictly between periscope-replay-direct-prod-[region]-public and audio-space
	re := regexp.MustCompile(`(periscope-replay-direct-prod-[^/]+-public/)[^/]+(/audio-space)`)
	mediaURL = re.ReplaceAllString(mediaURL, "${1}audio-space")

	logger.Debug("Parsed media playlist", "url", mediaURL)
	return mediaURL, nil
}

// resolveOverlappingPath concatenates baseURL and relPath, addressing any path overlaps.
// It prioritizes standard net/url logic for pure absolute or non-overlapping relative paths.
// But if there's overlap like baseURL = "https://host/a/b/c/" and relPath = "b/c/seg.ts",
// it stitches them cleanly as "https://host/a/b/c/seg.ts".
func resolveOverlappingPath(baseURL, relPath string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL %q: %w", baseURL, err)
	}
	rel, err := url.Parse(relPath)
	if err != nil {
		return "", fmt.Errorf("parse relative path %q: %w", relPath, err)
	}

	// If absolute path (starts with /), use standard URL referencing
	if strings.HasPrefix(relPath, "/") {
		return u.ResolveReference(rel).String(), nil
	}

	// For relative paths, perform overlap matching first
	relPathTrimmed := strings.TrimLeft(relPath, "/")
	baseParts := strings.Split(strings.TrimRight(baseURL, "/"), "/")
	relParts := strings.Split(relPathTrimmed, "/")

	// Attempt to find overlap by matching the leading chunk of relPath anywhere within baseURL
	for i := 0; i < len(baseParts); i++ {
		if baseParts[i] == relParts[0] {
			match := true
			overlap := len(baseParts) - i
			if overlap > len(relParts) {
				continue
			}
			for j := 0; j < overlap; j++ {
				if baseParts[i+j] != relParts[j] {
					match = false
					break
				}
			}
			if match {
				// We matched, assemble combining the overlapping regions
				return strings.Join(baseParts[:i], "/") + "/" + relPathTrimmed, nil
			}
		}
	}
	return u.ResolveReference(rel).String(), nil
}
