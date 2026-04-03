package internal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/jieri222/SpaceWatcher-Go/internal/logger"
	"github.com/jieri222/SpaceWatcher-Go/internal/m3u8"
)

const (
	DefaultWorkers = 5
	DefaultRetry   = 3
)

// Downloader handles the downloading process
type Downloader struct {
	session     *TwitterSession
	concurrency int
	retry       int
}

// NewDownloader creates a new Downloader
func NewDownloader(session *TwitterSession, concurrency int, retry int) *Downloader {
	if concurrency <= 0 {
		concurrency = DefaultWorkers
	}
	if retry <= 0 {
		retry = DefaultRetry
	}
	return &Downloader{
		session:     session,
		concurrency: concurrency,
		retry:       retry,
	}
}

// DownloadSpace performs the complete download flow (stream download + merge)
func (d *Downloader) DownloadSpace(ctx context.Context, m3u8URL string, metadata *SpaceMetadata, outputPath string) error {
	// Parse m3u8
	logger.Info("Parsing playlist...")
	playlist, err := m3u8.ParseM3U8(ctx, d.session.client, m3u8URL)
	if err != nil {
		return fmt.Errorf("parse m3u8: %w", err)
	}
	total := len(playlist.Segments)
	logger.Info("Found segments", "count", total)

	// Start ffmpeg
	logger.Info("Starting download", "concurrency", d.concurrency)
	title := metadata.Title
	if title == "" {
		title = fmt.Sprintf("%s's Space", metadata.CreatorResults.Result.Core.Name)
	}

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-loglevel", "error",
		"-y",        // Overwrite output
		"-f", "aac", // Specify input format
		"-i", "pipe:0", // Read from stdin
		"-c", "copy", // Lossless copy
		"-metadata", "title="+title,
		"-metadata", "artist="+metadata.CreatorResults.Result.Core.Name,
		outputPath,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create ffmpeg stdin pipe: %w", err)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	// Stream download and write
	downloadErr := d.streamDownloadAndMerge(ctx, playlist, stdin, total)

	// Close stdin to let ffmpeg exit
	stdin.Close()

	// Wait for ffmpeg
	ffmpegErr := cmd.Wait()

	// Prioritize download errors
	if downloadErr != nil {
		return downloadErr
	}
	if ffmpegErr != nil {
		// Context cancellation is not a failure
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg error: %w", ffmpegErr)
	}

	return nil
}

// SegmentResult represents the download result of a single segment
type SegmentResult struct {
	Index int
	Data  []byte
	Error error
}

// streamDownloadAndMerge streams downloads and writes to ffmpeg sequentially
func (d *Downloader) streamDownloadAndMerge(ctx context.Context, playlist *m3u8.Playlist, writer io.Writer, total int) error {
	resultChan := make(chan SegmentResult, d.concurrency*2)
	jobs := make(chan int, total)

	// Worker pool
	var wg sync.WaitGroup
	for i := 0; i < d.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case idx, ok := <-jobs:
					if !ok {
						return
					}
					seg := playlist.Segments[idx]
					segURL := seg.GetFullURL(playlist.BaseURL)

					data, err := d.downloadSegmentWithRetry(ctx, segURL, d.retry)
					select {
					case resultChan <- SegmentResult{Index: idx, Data: data, Error: err}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Send jobs
	go func() {
		for i := range playlist.Segments {
			select {
			case jobs <- i:
			case <-ctx.Done():
				close(jobs)
				return
			}
		}
		close(jobs)
	}()

	// Close resultChan
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Write sequentially - pending buffer handles out-of-order arrivals
	pending := make(map[int][]byte)
	nextExpected := 0
	completed := 0
	failedCount := 0

	for result := range resultChan {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		completed++
		if result.Error != nil {
			failedCount++
			logger.Warn("Segment download failed, skipped", "segment", result.Index, "error", result.Error)
			continue
		}

		// Buffer or write directly
		if result.Index == nextExpected {
			// Write directly
			if _, err := writer.Write(result.Data); err != nil {
				return fmt.Errorf("write segment %d to ffmpeg: %w", result.Index, err)
			}
			nextExpected++

			// Flush consecutive segments from pending buffer
			for {
				if data, ok := pending[nextExpected]; ok {
					if _, err := writer.Write(data); err != nil {
						return fmt.Errorf("write segment %d to ffmpeg: %w", nextExpected, err)
					}
					delete(pending, nextExpected)
					nextExpected++
				} else {
					break
				}
			}
		} else {
			// Add to pending buffer
			pending[result.Index] = result.Data
		}

		// Update every 10 segments or the last one
		if completed%10 == 0 || completed == total {
			// Use \r to reset cursor and \033[K to clear remaining characters on the line to avoid old content remaining
			fmt.Printf("\r\033[K%s \033[32m[INFO ]\033[0m Download progress \033[90m|\033[0m progress: %d/%d", time.Now().Format("2006-01-02 15:04:05-07"), completed, total)
			if completed == total {
				fmt.Println() // Newline after download completes
			}
		}
	}

	if failedCount > 0 {
		failRate := float64(failedCount) / float64(total)
		logger.Warn("Some segments failed to download", "failed", failedCount, "total", total)
		if failRate >= 0.05 {
			return fmt.Errorf("too many segment download failures: %d/%d (%.1f%%)", failedCount, total, failRate*100)
		}
	}

	return nil
}

// downloadSegment downloads a single segment
func (d *Downloader) downloadSegment(ctx context.Context, url string) ([]byte, error) {
	resp, err := d.session.client.Get(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("GET segment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("segment HTTP %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// downloadSegmentWithRetry downloads a segment with retries
func (d *Downloader) downloadSegmentWithRetry(ctx context.Context, url string, maxRetries int) ([]byte, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			// Exponential backoff: 1s, 2s, 4s...
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<i) * time.Second):
			}
		}
		data, err := d.downloadSegment(ctx, url)
		if err == nil {
			return data, nil
		}
		lastErr = err
		logger.Debug("Segment download failed, retrying", "attempt", i+1, "maxRetry", maxRetries, "error", err)
	}
	return nil, lastErr
}
