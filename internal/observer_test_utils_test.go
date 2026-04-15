package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// MockRoundTripper implements http.RoundTripper for mocking network calls
type MockRoundTripper struct {
	ResponseQueue map[string][]*http.Response
	CallCounts    map[string]int
}

func NewMockRoundTripper() *MockRoundTripper {
	return &MockRoundTripper{
		ResponseQueue: make(map[string][]*http.Response),
		CallCounts:    make(map[string]int),
	}
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	// Match based on simplified URL parts (e.g., endpoint name)
	var key string
	if strings.Contains(url, "AudioSpaceById") {
		key = "AudioSpaceById"
	} else if strings.Contains(url, "live_video_stream/status") {
		key = "StreamStatus"
	} else if strings.Contains(url, "master_playlist.m3u8") {
		key = "MasterPlaylist"
	} else {
		key = url
	}

	responses := m.ResponseQueue[key]
	count := m.CallCounts[key]

	if count >= len(responses) {
		// Default to 404 if no more responses queued
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}, nil
	}

	res := responses[count]
	m.CallCounts[key]++
	return res, nil
}

func (m *MockRoundTripper) AddAudioSpaceResponse(metadata *SpaceMetadata) {
	resp := &AudioSpaceByIdResponse{}
	resp.Data.AudioSpace.Metadata = *metadata
	body, _ := json.Marshal(resp)

	m.ResponseQueue["AudioSpaceById"] = append(m.ResponseQueue["AudioSpaceById"], &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBuffer(body)),
		Header:     make(http.Header),
	})
}

func (m *MockRoundTripper) AddStreamStatusResponse(statusCode int, location string) {
	var body string
	if statusCode == 200 {
		status := struct {
			Source struct {
				Location string `json:"location"`
			} `json:"source"`
		}{}
		status.Source.Location = location
		b, _ := json.Marshal(status)
		body = string(b)
	} else {
		body = "Error"
	}

	m.ResponseQueue["StreamStatus"] = append(m.ResponseQueue["StreamStatus"], &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	})
}

func (m *MockRoundTripper) AddMasterPlaylistResponse(content string) {
	m.ResponseQueue["MasterPlaylist"] = append(m.ResponseQueue["MasterPlaylist"], &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(content)),
		Header:     make(http.Header),
	})
}

func createTestObserver(mock *MockRoundTripper) *Observer {
	s := NewTwitterSession()
	s.client.HTTPClient.Transport = mock
	s.queryID = "test-query-id"
	s.guestToken = "test-guest-token"
	return NewObserver(s, 10*time.Millisecond, 3)
}
