package main

import (
	"context"
	"fmt"
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
	// CLI arguments
	spaceURL := flag.String("url", "", "Space URL or ID")
	output := flag.StringP("output", "o", internal.DefaultFilenameFormat, "Output file path, supports format variables: {date}, {time}, {datetime}, {title}, {creator_name}, {creator_screen_name}, {spaceID}")
	concurrency := flag.IntP("concurrency", "c", internal.DefaultWorkers, "Download concurrency")
	retry := flag.IntP("retry", "r", internal.DefaultRetry, "Download/wait retry count")
	interval := flag.IntP("interval", "i", 30, "Monitor interval (seconds)")
	verbose := flag.BoolP("verbose", "v", false, "Show verbose log")
	flag.Parse()

	// Initialize Logger
	logger.InitLogger(*verbose)

	// Setup Ctrl+C handler
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		logger.Warn("Received interrupt signal, shutting down...")
		cancel()
	}()

	// Get URL from remaining arguments
	if *spaceURL == "" && len(flag.Args()) > 0 {
		*spaceURL = flag.Args()[0]
	}

	if *spaceURL == "" {
		fmt.Fprintln(os.Stderr, "Usage: spacewatcher [options] <space_url_or_id>")
		fmt.Fprintln(os.Stderr, "Note: all options must be placed before the URL")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  spacewatcher https://x.com/i/spaces/xxxxxxxxxxxxx")
		fmt.Fprintln(os.Stderr, "  spacewatcher -o space.m4a https://x.com/i/spaces/xxxxx")
		fmt.Fprintln(os.Stderr, "  spacewatcher -o \"{date}_{creator_name}.m4a\" https://x.com/i/spaces/xxxxx")
		fmt.Fprintln(os.Stderr, "  spacewatcher -v https://x.com/i/spaces/xxxxx")
		os.Exit(1)
	}

	// Parse Space ID
	spaceID := parseSpaceID(*spaceURL)
	if spaceID == "" {
		logger.Error("Failed to parse Space ID", "input", *spaceURL)
		os.Exit(1)
	}
	logger.Info("Parsed Space ID", "id", spaceID)

	// Initialize Session
	logger.Info("Initializing session...")
	session := internal.NewTwitterSession()
	if err := session.RefreshGuestToken(); err != nil {
		logger.Error("Failed to get Guest Token", "error", err)
		os.Exit(1)
	}
	logger.Debug("Got Guest Token", "token", session.GetGuestToken())

	// Get QueryID
	logger.Info("Getting API Query ID...")
	if err := session.DiscoverQueryID(); err != nil {
		logger.Error("Failed to get Query ID", "error", err)
		os.Exit(1)
	}
	logger.Debug("Got Query ID", "id", session.GetQueryID())

	// Wait for Space and get stream info
	observer := internal.NewObserver(session, time.Duration(*interval)*time.Second, *retry)
	result, err := observer.Resolve(ctx, spaceID)
	if err != nil {
		if ctx.Err() != nil {
			logger.Warn("Cancelled")
			os.Exit(0)
		}
		logger.Error("Failed to get Space", "error", err)
		os.Exit(1)
	}
	metadata := result.Metadata
	m3u8URL := result.M3U8URL

	// Determine output filename
	formatter := internal.NewFilenameFormatter(*output)
	outputPath := formatter.Format(metadata)
	logger.Debug("Output file", "path", outputPath)

	// Download
	logger.Info("Starting download...")
	downloader := internal.NewDownloader(session, *concurrency, *retry)
	if err := downloader.DownloadSpace(ctx, m3u8URL, metadata, outputPath); err != nil {
		if ctx.Err() != nil {
			logger.Warn("Download cancelled")
			os.Exit(0)
		}
		logger.Error("Download failed", "error", err)
		os.Exit(1)
	}

	logger.Info("✅ Download completed", "output", outputPath)
}

// parseSpaceID parses Space ID from URL or direct ID
func parseSpaceID(input string) string {
	input = strings.TrimSpace(input)

	// If it's directly an ID
	if !strings.Contains(input, "/") {
		return input
	}

	// URL format: https://x.com/i/spaces/xxxxxxxxxxxxx or twitter.com variants
	pattern := `(?:twitter\.com|x\.com)/i/spaces/([a-zA-Z0-9]+)`
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(input)
	if len(match) > 1 {
		return match[1]
	}

	return ""
}
