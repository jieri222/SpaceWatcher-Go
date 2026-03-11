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

// GetMasterPlaylistURL 將 dynamic playlist URL 轉換為 master playlist URL
func GetMasterPlaylistURL(ctx context.Context, client *client.Client, mediaKey string) (string, error) {
	dynamicURL, err := GetSourceLocation(ctx, client, mediaKey)
	if err != nil {
		return "", err
	}
	baseUrl := strings.Split(dynamicURL, "/")
	baseUrl = baseUrl[:len(baseUrl)-1]
	return strings.Join(baseUrl, "/") + "/master_playlist.m3u8", nil
}

// ResolveMasterPlaylist 解析 master playlist，取得裡面的 media playlist URL
// master playlist 裡會列出不同 bandwidth 的 variant，這裡取第一個
func ResolveMasterPlaylist(ctx context.Context, client *client.Client, masterURL string) (string, error) {
	resp, err := client.Get(ctx, masterURL, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("master playlist HTTP error %d: %s", resp.StatusCode, string(body))
	}

	playlist, _, err := m3u8.DecodeFrom(strings.NewReader(string(body)), false)
	if err != nil {
		return "", err
	}

	master, ok := playlist.(*m3u8.MasterPlaylist)
	if !ok {
		// 如果已經是 media playlist，直接回傳 masterURL 本身
		return masterURL, nil
	}

	variants := master.Variants
	if len(variants) == 0 {
		return "", fmt.Errorf("master playlist has no variants")
	}

	// 取第一個 variant 的 URI
	variantURI := variants[0].URI
	if strings.HasPrefix(variantURI, "http") {
		return variantURI, nil
	}

	// 相對路徑，拼接 base URL（處理重疊路徑）
	baseURL := masterURL[:strings.LastIndex(masterURL, "/")+1]
	mediaURL := resolveOverlappingPath(baseURL, variantURI)

	// 把 transcode 改成 non_transcode
	mediaURL = strings.Replace(mediaURL, "transcode", "non_transcode", 1)

	// 刪除 JWT token：periscope-replay-direct-prod-[region]-public 之後與 audio-space 之間的部分
	re := regexp.MustCompile(`(periscope-replay-direct-prod-[^/]+-public/)[^/]+(/audio-space)`)
	mediaURL = re.ReplaceAllString(mediaURL, "${1}audio-space")

	logger.Debug("解析出 media playlist", "url", mediaURL)
	return mediaURL, nil
}

// resolveOverlappingPath 拼接 baseURL 和 relPath，處理路徑重疊
// 優先使用 net/url 處理標準的絕對路徑或無重疊相對路徑
// 若為相對路徑且存在重疊段，例如 baseURL = "https://host/a/b/c/" 且 relPath = "b/c/seg.ts"
// 會找到重疊的 "b/c/"，回傳 "https://host/a/b/c/seg.ts"
func resolveOverlappingPath(baseURL, relPath string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	rel, err := url.Parse(relPath)
	if err != nil {
		return ""
	}

	// 如果是絕對路徑（以 / 開頭），直接使用標準 URL 解析組合
	if strings.HasPrefix(relPath, "/") {
		return u.ResolveReference(rel).String()
	}

	// 對於相對路徑，先嘗試重疊邏輯
	relPathTrimmed := strings.TrimLeft(relPath, "/")
	baseParts := strings.Split(strings.TrimRight(baseURL, "/"), "/")
	relParts := strings.Split(relPathTrimmed, "/")

	// 嘗試從 relPath 的第一段在 baseURL 中找到匹配位置
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
				// 從重疊處開始拼接
				return strings.Join(baseParts[:i], "/") + "/" + relPathTrimmed
			}
		}
	}
	return u.ResolveReference(rel).String()
}
