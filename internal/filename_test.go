package internal

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewFilenameFormatter(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   string
	}{
		{
			name:   "Default Format",
			format: "",
			want:   DefaultFilenameFormat,
		},
		{
			name:   "Custom Format",
			format: "{title}.mp3",
			want:   "{title}.mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilenameFormatter(tt.format)
			if f.format != tt.want {
				t.Errorf("NewFilenameFormatter() format = %v, want %v", f.format, tt.want)
			}
		})
	}
}

func TestFilenameFormatter_Format(t *testing.T) {
	// Fixed time for deterministic testing: 2024-03-21 15:04:05 UTC
	fixedTime := time.Date(2024, 3, 21, 15, 4, 5, 0, time.UTC)
	startedAt := fixedTime.UnixMilli()

	metadata := &SpaceMetadata{
		RestID:    "123456789",
		Title:     "Test Space :)",
		StartedAt: startedAt,
	}
	metadata.CreatorResults.Result.Core.Name = "Test User"
	metadata.CreatorResults.Result.Core.ScreenName = "testuser"

	tests := []struct {
		name     string
		format   string
		metadata *SpaceMetadata
		want     string
	}{
		{
			name:     "Standard placeholders",
			format:   "{date}_{time}_{title}_{creator_name}_{creator_screen_name}_{spaceID}.m4a",
			metadata: metadata,
			// Title "Test Space :)" becomes "Test Space ꞉)"
			want: "20240321_150405_Test Space ꞉)_Test User_@testuser_123456789.m4a",
		},
		{
			name:     "Datetime placeholder",
			format:   "{datetime}.m4a",
			metadata: metadata,
			want:     "20240321_150405.m4a",
		},
		{
			name:   "Empty title fallback",
			format: "{title}.m4a",
			metadata: func() *SpaceMetadata {
				m := &SpaceMetadata{StartedAt: startedAt}
				m.CreatorResults.Result.Core.Name = "EmptyTitleUser"
				return m
			}(),
			want: "EmptyTitleUser's Space.m4a",
		},
		{
			name:   "Path preservation and placeholder sanitization",
			format: "recording/{title}.m4a",
			metadata: &SpaceMetadata{
				Title: "Title/With/Slashes",
			},
			want: filepath.Join("recording", "Title∕With∕Slashes.m4a"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilenameFormatter(tt.format)
			got := f.Format(tt.metadata)
			// Normalize slashes for comparison
			if filepath.ToSlash(got) != filepath.ToSlash(tt.want) {
				t.Errorf("Format() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Illegal Windows characters to Full-width",
			input:    `foo/bar\baz:qux*quux?corge"grault<garply>waldo|fred`,
			expected: "foo∕bar⧵baz꞉qux∗quux？corge″grault˂garply˃waldo⏐fred",
		},
		{
			name:     "Remove control characters",
			input:    "line1\nline2\r\t tab",
			expected: "line1line2 tab",
		},
		{
			name:     "Trim whitespace",
			input:    "  filename with spaces  ",
			expected: "filename with spaces",
		},
		{
			name:     "Empty result fallback",
			input:    "\x01\x02",
			expected: "space",
		},
		{
			name:     "Normal filename",
			input:    "normal_file.txt",
			expected: "normal_file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeFilename_Fallback(t *testing.T) {
	// Special case for truly empty names
	got := sanitizeFilename("")
	if got != "space" {
		t.Errorf("sanitizeFilename(\"\") = %q, want \"space\"", got)
	}
}
