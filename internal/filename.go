package internal

import (
	"fmt"
	"path/filepath"
	"regexp"
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
		startTime = time.UnixMilli(metadata.StartedAt)
	} else {
		startTime = time.Now()
	}

	// Title fallback
	title := metadata.Title
	if title == "" {
		title = fmt.Sprintf("%s's Space", metadata.CreatorResults.Result.Core.Name)
	}

	// Prepare replacement variables
	replacements := map[string]string{
		"{date}":                startTime.Format("20060102"),
		"{time}":                startTime.Format("150405"),
		"{datetime}":            startTime.Format("20060102_150405"),
		"{title}":               title,
		"{creator_name}":        metadata.CreatorResults.Result.Core.Name,
		"{creator_screen_name}": fmt.Sprintf("@%s", metadata.CreatorResults.Result.Core.ScreenName),
		"{spaceID}":             metadata.RestID,
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
	// Windows does not allow: / \ : * ? " < > |
	// Nor does it allow control characters 0-31
	illegalChars := regexp.MustCompile(`[/\\:*?"<>|]`)
	result := illegalChars.ReplaceAllString(name, "_")

	// Remove leading/trailing whitespaces
	result = strings.TrimSpace(result)

	// Prevent empty filenames
	if result == "" {
		result = "space"
	}

	return result
}
