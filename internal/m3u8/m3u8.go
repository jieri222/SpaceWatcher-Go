package m3u8

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jieri222/SpaceWatcher-Go/internal/client"

	"github.com/Eyevinn/hls-m3u8/m3u8"
)

const BaseStreamAPI = "https://api.x.com/1.1/live_video_stream/status/"

// ErrStreamNotFound indicates that the stream status API returned a 404
var ErrStreamNotFound = errors.New("stream status API error 404")

// StreamStatus represents the live_video_stream/status response
type StreamStatus struct {
	Source struct {
		Location              string `json:"location"`
		NoRedirectPlaybackUrl string `json:"noRedirectPlaybackUrl"`
		Status                string `json:"status"`
		StreamType            string `json:"streamType"`
	} `json:"source"`
	SessionID      string `json:"sessionId"`
	ChatToken      string `json:"chatToken"`
	LifecycleToken string `json:"lifecycleToken"`
	ShareUrl       string `json:"shareUrl"`
}

// GetSourceLocation
// If the Space has not ended, it returns the dynamic playlist URL.
// If the Space has ended, it returns the final m3u8 URL.
func GetSourceLocation(ctx context.Context, client *client.Client, mediaKey string) (string, error) {
	url := BaseStreamAPI + mediaKey

	resp, err := client.Get(ctx, url, nil)
	if err != nil {
		return "", fmt.Errorf("get stream status for %s: %w", mediaKey, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read stream status response: %w", err)
	}

	switch resp.StatusCode {
	case 200:
		break
	case 404:
		return "", ErrStreamNotFound
	default:
		return "", fmt.Errorf("stream status API error %d: %s", resp.StatusCode, string(body))
	}

	var status StreamStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return "", fmt.Errorf("decode stream status JSON: %w", err)
	}

	if status.Source.Location == "" {
		return "", fmt.Errorf("no stream location found in response")
	}

	return status.Source.Location, nil
}

// Segment represents an m3u8 segment
type Segment struct {
	URI      string
	Duration float64
}

// Playlist represents an m3u8 playlist
type Playlist struct {
	Segments []*Segment
	BaseURL  string
}

// ParseM3U8 parses an m3u8 URL and returns a Playlist
func ParseM3U8(ctx context.Context, client *client.Client, m3u8URL string) (*Playlist, error) {
	resp, err := client.Get(ctx, m3u8URL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch m3u8 playlist: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read m3u8 playlist body: %w", err)
	}

	playlist, _, err := m3u8.DecodeFrom(strings.NewReader(string(body)), false)
	if err != nil {
		return nil, fmt.Errorf("decode m3u8 playlist: %w", err)
	}

	// Calculate base URL
	baseURL := m3u8URL[:strings.LastIndex(m3u8URL, "/")+1]

	// Media playlist
	media, ok := playlist.(*m3u8.MediaPlaylist)
	if !ok {
		return nil, fmt.Errorf("expected media playlist, got master playlist")
	}
	rawSegments := media.GetAllSegments()

	// Convert to our custom Segment type
	segments := make([]*Segment, len(rawSegments))
	for i, seg := range rawSegments {
		segments[i] = &Segment{
			URI:      seg.URI,
			Duration: seg.Duration,
		}
	}

	return &Playlist{
		Segments: segments,
		BaseURL:  baseURL,
	}, nil
}

// GetFullURL returns the complete URL of the segment
func (s *Segment) GetFullURL(baseURL string) string {
	if strings.HasPrefix(s.URI, "http") {
		return s.URI
	}
	return baseURL + s.URI
}
