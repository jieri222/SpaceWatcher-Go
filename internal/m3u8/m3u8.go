package m3u8

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jieri222/SpaceWatcher-Go/internal/client"

	"github.com/Eyevinn/hls-m3u8/m3u8"
)

const BaseStreamAPI = "https://api.x.com/1.1/live_video_stream/status/"

// StreamStatus live_video_stream/status 回應
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
// 若 Space 尚未結束，會回傳 dynamic playlist URL
// 若 Space 已結束，會回傳最終的 m3u8 URL
func GetSourceLocation(ctx context.Context, client *client.Client, mediaKey string) (string, error) {
	url := BaseStreamAPI + mediaKey

	resp, err := client.Get(ctx, url, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("stream status API error %d: %s", resp.StatusCode, string(body))
	}

	var status StreamStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return "", err
	}

	if status.Source.Location == "" {
		return "", fmt.Errorf("no stream location found in response")
	}

	return status.Source.Location, nil
}

// Segment 代表一個 m3u8 segment
type Segment struct {
	URI      string
	Duration float64
}

// Playlist m3u8 播放清單
type Playlist struct {
	Segments []*Segment
	BaseURL  string
}

// ParseM3U8 解析 m3u8 URL 並返回 Playlist
func ParseM3U8(ctx context.Context, client *client.Client, m3u8URL string) (*Playlist, error) {
	resp, err := client.Get(ctx, m3u8URL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	playlist, _, err := m3u8.DecodeFrom(strings.NewReader(string(body)), false)
	if err != nil {
		return nil, err
	}

	// 計算 base URL
	baseURL := m3u8URL[:strings.LastIndex(m3u8URL, "/")+1]

	// Media playlist
	media, ok := playlist.(*m3u8.MediaPlaylist)
	if !ok {
		return nil, fmt.Errorf("expected media playlist, got master playlist")
	}
	rawSegments := media.GetAllSegments()

	// 轉換成我們的 Segment 類型
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

// GetFullURL 取得 segment 的完整 URL
func (s *Segment) GetFullURL(baseURL string) string {
	if strings.HasPrefix(s.URI, "http") {
		return s.URI
	}
	return baseURL + s.URI
}
