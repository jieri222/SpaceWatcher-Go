package internal

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultFilenameFormat 預設檔名格式
	DefaultFilenameFormat = "{date}_{title}.m4a"
)

// FilenameFormatter 檔名格式化器
type FilenameFormatter struct {
	format string
}

// NewFilenameFormatter 建立格式化器
func NewFilenameFormatter(format string) *FilenameFormatter {
	Debug("使用格式", "format", format)
	if format == "" {
		format = DefaultFilenameFormat
	}
	return &FilenameFormatter{format: format}
}

// Format 根據 metadata 生成檔名
// 支援的變數: {date}, {time}, {datetime}, {title}, {creator_name}, {creator_screen_name}, {spaceID}
func (f *FilenameFormatter) Format(metadata *SpaceMetadata) string {
	// 取得時間資訊
	var startTime time.Time
	if metadata.StartedAt > 0 {
		startTime = time.UnixMilli(metadata.StartedAt)
	} else {
		startTime = time.Now()
	}

	// 準備替換變數
	replacements := map[string]string{
		"{date}":                startTime.Format("20060102"),
		"{time}":                startTime.Format("150405"),
		"{datetime}":            startTime.Format("20060102_150405"),
		"{title}":               metadata.Title,
		"{creator_name}":        metadata.CreatorResults.Result.Core.Name,
		"{creator_screen_name}": fmt.Sprintf("@%s", metadata.CreatorResults.Result.Core.ScreenName),
		"{spaceID}":             metadata.RestID,
	}

	result := f.format
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// 清理 Windows 非法字元
	if runtime.GOOS == "windows" {
		result = sanitizeFilename(result)
	}

	return result
}

// sanitizeFilename 移除 Windows 檔名中的非法字元
func sanitizeFilename(name string) string {
	// Windows 不允許: / \ : * ? " < > |
	// 還有控制字元 0-31
	illegalChars := regexp.MustCompile(`[/\\:*?"<>|]`)
	result := illegalChars.ReplaceAllString(name, "_")

	// 移除開頭結尾空白
	result = strings.TrimSpace(result)

	// 避免空檔名
	if result == "" {
		result = "space"
	}

	return result
}
