package internal

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestObserver_waitUntilEnded(t *testing.T) {
	tests := []struct {
		name      string
		initial   *SpaceMetadata
		setupMock func(*MockRoundTripper)
		want      *WaitResult
		wantErr   bool
		errCheck  func(error) bool
	}{
		{
			name: "Success - Standard Running to Ended",
			initial: &SpaceMetadata{
				State:    StateRunning,
				MediaKey: "live-key",
			},
			setupMock: func(m *MockRoundTripper) {
				// 1. fetchMasterURL in loop (1st iteration)
				m.AddStreamStatusResponse(200, "https://stream/live-key/playlist.m3u8")
				// 2. fetchStatus in ticker (1st tick) -> Ended
				m.AddAudioSpaceResponse(&SpaceMetadata{State: StateEnded, MediaKey: "live-key"})
				// 3. ResolveMasterPlaylist
				m.AddMasterPlaylistResponse("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1280000\nchunked.m3u8")
			},
			want: &WaitResult{
				SpaceID:    "test-space",
				FinalState: StateEnded,
				M3U8URL:    "https://stream/live-key/chunked.m3u8",
			},
			wantErr: false,
		},
		{
			name: "Success - NotStarted to Running to Ended",
			initial: &SpaceMetadata{
				State:    StateNotStarted,
				MediaKey: "waiting-key",
			},
			setupMock: func(m *MockRoundTripper) {
				// 1. fetchMasterURL in loop (1st iteration) -> 404
				m.AddStreamStatusResponse(404, "")
				// 2. fetchStatus in ticker (1st tick) -> Running
				m.AddAudioSpaceResponse(&SpaceMetadata{State: StateRunning, MediaKey: "waiting-key"})
				// 3. fetchMasterURL in loop (2nd iteration) -> 200
				m.AddStreamStatusResponse(200, "https://stream/waiting-key/playlist.m3u8")
				// 4. fetchStatus in ticker (2nd tick) -> Ended
				m.AddAudioSpaceResponse(&SpaceMetadata{State: StateEnded, MediaKey: "waiting-key"})
				// 5. ResolveMasterPlaylist
				m.AddMasterPlaylistResponse("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1280000\nchunked.m3u8")
			},
			want: &WaitResult{
				SpaceID:    "test-space",
				FinalState: StateEnded,
				M3U8URL:    "https://stream/waiting-key/chunked.m3u8",
			},
			wantErr: false,
		},
		{
			name: "Success - Replay recovery after missed live",
			initial: &SpaceMetadata{
				State:    StateRunning,
				MediaKey: "missed-key",
			},
			setupMock: func(m *MockRoundTripper) {
				// 1. fetchMasterURL in loop (1st iteration) -> 404
				m.AddStreamStatusResponse(404, "")
				// 2. fetchStatus in ticker (1st tick) -> Ended (supports replay)
				m.AddAudioSpaceResponse(&SpaceMetadata{
					State:                     StateEnded,
					MediaKey:                  "missed-key",
					IsSpaceAvailableforReplay: true,
				})
				// 3. fetchMasterURL in Ended block (last attempt) -> 200
				m.AddStreamStatusResponse(200, "https://stream/missed-key/playlist.m3u8")
				// 4. ResolveMasterPlaylist
				m.AddMasterPlaylistResponse("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1280000\nchunked.m3u8")
			},
			want: &WaitResult{
				SpaceID:    "test-space",
				FinalState: StateEnded,
				M3U8URL:    "https://stream/missed-key/chunked.m3u8",
			},
			wantErr: false,
		},
		{
			name: "Failure - Fatal Error (401 Unauthorized)",
			initial: &SpaceMetadata{
				State:    StateRunning,
				MediaKey: "bad-key",
			},
			setupMock: func(m *MockRoundTripper) {
				// 1. fetchMasterURL in loop (1st iteration) -> 401
				m.AddStreamStatusResponse(401, "")
			},
			wantErr: true,
			errCheck: func(err error) bool {
				return strings.Contains(err.Error(), "401")
			},
		},
		{
			name: "Failure - Status Retrieval Exceeds Max Retries",
			initial: &SpaceMetadata{
				State:    StateRunning,
				MediaKey: "retry-key",
			},
			setupMock: func(m *MockRoundTripper) {
				// 1. fetchMasterURL succeeds
				m.AddStreamStatusResponse(200, "https://stream/retry-key/playlist.m3u8")
				// 2. fetchStatus fails consistently (3 times)
				m.ResponseQueue["AudioSpaceById"] = []*http.Response{
					{StatusCode: 500, Body: io.NopCloser(bytes.NewBufferString("Fail")), Header: make(http.Header)},
					{StatusCode: 500, Body: io.NopCloser(bytes.NewBufferString("Fail")), Header: make(http.Header)},
					{StatusCode: 500, Body: io.NopCloser(bytes.NewBufferString("Fail")), Header: make(http.Header)},
				}
			},
			wantErr: true,
			errCheck: func(err error) bool {
				return strings.Contains(err.Error(), "retry limit")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockRoundTripper()
			if tt.setupMock != nil {
				tt.setupMock(mock)
			}
			// Use short interval for testing
			o := createTestObserver(mock)
			o.interval = 1 * time.Millisecond

			got, err := o.waitUntilEnded(context.Background(), "test-space", tt.initial)

			if (err != nil) != tt.wantErr {
				t.Fatalf("waitUntilEnded() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errCheck != nil && !tt.errCheck(err) {
				t.Errorf("waitUntilEnded() error %v did not pass error check", err)
			}
			if !tt.wantErr {
				if got.SpaceID != tt.want.SpaceID || got.FinalState != tt.want.FinalState || got.M3U8URL != tt.want.M3U8URL {
					t.Errorf("waitUntilEnded() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
