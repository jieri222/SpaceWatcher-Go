package internal

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jieri222/SpaceWatcher-Go/internal/logger"
)

const (
	// DefaultFilenameFormat is the fallback filename format
	DefaultFilenameFormat = "{date}_{title}.m4a"
)

// FilenameFormatter formats output filenames
type FilenameFormatter struct {
	format string
}

// NewFilenameFormatter creates a new FilenameFormatter
func NewFilenameFormatter(format string) *FilenameFormatter {
	if format == "" {
		format = DefaultFilenameFormat
	}
	logger.Debug("Applied filename format", "format", format)
	return &FilenameFormatter{format: format}
}

// Format generates a filename based on Space metadata
// Supported variables: {date}, {time}, {datetime}, {title}, {creator_name}, {creator_screen_name}, {spaceID}
func (f *FilenameFormatter) Format(metadata *SpaceMetadata) string {
	// Extract time information
	var startTime time.Time
	if metadata.StartedAt > 0 {
		startTime = time.UnixMilli(metadata.StartedAt).UTC()
	} else {
		startTime = time.Now().UTC()
	}

	// Title fallback
	title := metadata.Title
	if title == "" {
		title = fmt.Sprintf("%s's Space", metadata.CreatorResults.Result.Core.Name)
	}

	// Prepare replacement variables (sanitized to prevent breaking directory structure)
	replacements := map[string]string{
		"{date}":                startTime.Format("20060102"),
		"{time}":                startTime.Format("150405"),
		"{datetime}":            startTime.Format("20060102_150405"),
		"{title}":               sanitizeFilename(title),
		"{creator_name}":        sanitizeFilename(metadata.CreatorResults.Result.Core.Name),
		"{creator_screen_name}": sanitizeFilename(fmt.Sprintf("@%s", metadata.CreatorResults.Result.Core.ScreenName)),
		"{spaceID}":             sanitizeFilename(metadata.RestID),
	}

	result := f.format
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// Clean out invalid characters from the filename (preserves directory path)
	// Execute on all platforms for cross-platform compatibility
	dir := filepath.Dir(result)
	filename := filepath.Base(result)
	filename = sanitizeFilename(filename)
	if dir != "." {
		result = filepath.Join(dir, filename)
	} else {
		result = filename
	}

	return result
}

// sanitizeFilename removes characters that are unsupported by Windows
func sanitizeFilename(name string) string {
	// Map illegal Windows characters to their full-width equivalents
	// This preserves the visual look while ensuring filesystem compatibility
	replacer := strings.NewReplacer(
		"/", "∕", // U+2215
		"\\", "⧵", // U+29F5
		":", "꞉", // U+A789
		"*", "∗", // U+2217
		"?", "？", // U+ff1f
		"\"", "″", // U+2033
		"<", "˂", // U+02C2
		">", "˃", // U+02C3
		"|", "⏐", // U+23D0
	)
	result := replacer.Replace(name)

	// Remove control characters (0-31) and 127 (DEL)
	result = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1 // Drop the character
		}
		return r
	}, result)

	// Remove leading/trailing whitespaces
	result = strings.TrimSpace(result)

	// Prevent empty filenames
	if result == "" {
		result = "space"
	}

	return result
}
