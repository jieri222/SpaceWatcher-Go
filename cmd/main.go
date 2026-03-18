package main

import (
	"context"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"github.com/jieri222/SpaceWatcher-Go/internal"
	"github.com/jieri222/SpaceWatcher-Go/internal/logger"

	flag "github.com/spf13/pflag"
)

func main() {
	// CLI 參數
	spaceURL := flag.String("url", "", "Space URL 或 ID")
	output := flag.StringP("output", "o", internal.DefaultFilenameFormat, "輸出檔案路徑，支援格式變數: {date}, {time}, {datetime}, {title}, {creator_name}, {creator_screen_name}, {spaceID}")
	concurrency := flag.IntP("concurrency", "c", internal.DefaultWorkers, "下載併發數")
	retry := flag.IntP("retry", "r", internal.DefaultRetry, "下載/等待重試次數")
	interval := flag.IntP("interval", "i", 30, "監控間隔 (秒)")
	verbose := flag.BoolP("verbose", "v", false, "顯示詳細 log")
	flag.Parse()

	// 初始化 Logger
	logger.InitLogger(*verbose)

	// 設定 Ctrl+C 處理
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		logger.Warn("收到中斷信號，正在關閉...")
		cancel()
	}()

	// 從剩餘參數取 URL
	if *spaceURL == "" && len(flag.Args()) > 0 {
		*spaceURL = flag.Args()[0]
	}

	if *spaceURL == "" {
		logger.Error("用法: spacewatcher [選項] <space_url_or_id>")
		logger.Info("注意: 所有選項必須放在 URL 之前")
		logger.Info("")
		logger.Info("範例:")
		logger.Info("  spacewatcher https://x.com/i/spaces/xxxxxxxxxxxxx")
		logger.Info("  spacewatcher -o space.m4a https://x.com/i/spaces/xxxxx")
		logger.Info("  spacewatcher -o \"{date}_{creator_name}.m4a\" https://x.com/i/spaces/xxxxx")
		logger.Info("  spacewatcher -v https://x.com/i/spaces/xxxxx")
		os.Exit(1)
	}

	// 解析 Space ID
	spaceID := parseSpaceID(*spaceURL)
	if spaceID == "" {
		logger.Error("無法解析 Space ID", "input", *spaceURL)
		os.Exit(1)
	}
	logger.Info("已解析 Space ID", "id", spaceID)

	// 初始化 Session
	logger.Info("初始化 Session...")
	session := internal.NewTwitterSession()
	if err := session.RefreshGuestToken(); err != nil {
		logger.Error("取得 Guest Token 失敗", "error", err)
		os.Exit(1)
	}
	logger.Debug("取得 Guest Token", "token", session.GetGuestToken())

	// 取得 QueryID
	logger.Info("取得 API Query ID...")
	if err := session.DiscoverQueryID(); err != nil {
		logger.Error("取得 QueryID 失敗", "error", err)
		os.Exit(1)
	}
	logger.Debug("取得 Query ID", "id", session.GetQueryID())

	// 取得 Space 資訊並等待結束
	observer := internal.NewObserver(session, time.Duration(*interval)*time.Second, *retry)
	result, err := observer.Resolve(ctx, spaceID)
	if err != nil {
		if ctx.Err() != nil {
			logger.Warn("已取消")
			os.Exit(0)
		}
		logger.Error("取得 Space 失敗", "error", err)
		os.Exit(1)
	}
	metadata := result.Metadata
	m3u8URL := result.M3U8URL

	// 決定輸出檔名
	formatter := internal.NewFilenameFormatter(*output)
	outputPath := formatter.Format(metadata)
	logger.Debug("輸出檔案", "path", outputPath)

	// 下載
	logger.Info("開始下載...")
	downloader := internal.NewDownloader(session, *concurrency, *retry)
	if err := downloader.DownloadSpace(ctx, m3u8URL, metadata, outputPath); err != nil {
		if ctx.Err() != nil {
			logger.Warn("已取消下載")
			os.Exit(0)
		}
		logger.Error("下載失敗", "error", err)
		os.Exit(1)
	}

	logger.Info("✅ 下載完成", "output", outputPath)
}

// parseSpaceID 從 URL 或直接的 ID 解析出 Space ID
func parseSpaceID(input string) string {
	input = strings.TrimSpace(input)

	// 直接是 ID 的情況
	if !strings.Contains(input, "/") {
		return input
	}

	// URL 格式: https://x.com/i/spaces/xxxxxxxxxxxxx 或 twitter.com 變體
	pattern := `(?:twitter\.com|x\.com)/i/spaces/([a-zA-Z0-9]+)`
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(input)
	if len(match) > 1 {
		return match[1]
	}

	return ""
}
