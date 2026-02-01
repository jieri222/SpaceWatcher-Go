package internal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	DefaultWorkers = 5
	DefaultRetry   = 3
)

// Downloader 下載器
type Downloader struct {
	session     *TwitterSession
	concurrency int
	retry       int
}

// NewDownloader 建立下載器
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

// DownloadSpace 完整下載流程 (串流下載+合併)
func (d *Downloader) DownloadSpace(ctx context.Context, m3u8URL string, metadata *SpaceMetadata, outputPath string) error {
	// 解析 m3u8
	Info("解析播放清單...")
	playlist, err := ParseM3U8(ctx, d.session.client, m3u8URL)
	if err != nil {
		return fmt.Errorf("failed to parse m3u8: %w", err)
	}
	total := len(playlist.Segments)
	Info("找到 segments", "count", total)

	// 啟動 ffmpeg
	Info("開始下載", "concurrency", d.concurrency)
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-loglevel", "error",
		"-y",        // 覆蓋輸出
		"-f", "aac", // 指定輸入格式
		"-i", "pipe:0", // 從 stdin 讀取
		"-c", "copy", // 無損複製
		"-metadata", "title="+metadata.Title,
		"-metadata", "artist="+metadata.CreatorResults.Result.Core.Name,
		outputPath,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// 串流下載並寫入
	downloadErr := d.streamDownloadAndMerge(ctx, playlist, stdin, total)

	// 關閉 stdin 讓 ffmpeg 結束
	stdin.Close()

	// 等待 ffmpeg
	ffmpegErr := cmd.Wait()

	// 優先返回下載錯誤
	if downloadErr != nil {
		return downloadErr
	}
	if ffmpegErr != nil {
		// Context 取消造成的錯誤不算失敗
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg error: %w", ffmpegErr)
	}

	return nil
}

// SegmentResult 下載結果
type SegmentResult struct {
	Index int
	Data  []byte
	Error error
}

// streamDownloadAndMerge 串流下載並按順序寫入 ffmpeg
func (d *Downloader) streamDownloadAndMerge(ctx context.Context, playlist *Playlist, writer io.Writer, total int) error {
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

	// 發送任務
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

	// 關閉 resultChan
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 按順序寫入 - pending buffer 處理亂序到達
	pending := make(map[int][]byte)
	nextExpected := 0
	completed := 0
	var downloadErrors []string

	for result := range resultChan {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		completed++
		if result.Error != nil {
			downloadErrors = append(downloadErrors, fmt.Sprintf("segment %d: %v", result.Index, result.Error))
			continue
		}

		// 暫存或直接寫入
		if result.Index == nextExpected {
			// 直接寫入
			if _, err := writer.Write(result.Data); err != nil {
				return fmt.Errorf("failed to write segment %d: %w", result.Index, err)
			}
			nextExpected++

			// Flush pending 中連續的 segments
			for {
				if data, ok := pending[nextExpected]; ok {
					if _, err := writer.Write(data); err != nil {
						return fmt.Errorf("failed to write segment %d: %w", nextExpected, err)
					}
					delete(pending, nextExpected)
					nextExpected++
				} else {
					break
				}
			}
		} else {
			// 暫存
			pending[result.Index] = result.Data
		}

		if completed%50 == 0 || completed == total {
			Debug("下載進度", "completed", fmt.Sprintf("%d/%d", completed, total))
		}
	}

	if len(downloadErrors) > 0 {
		return fmt.Errorf("download errors: %s", strings.Join(downloadErrors, "; "))
	}

	return nil
}

// downloadSegment 下載單個 segment
func (d *Downloader) downloadSegment(ctx context.Context, url string) ([]byte, error) {
	resp, err := d.session.client.Get(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// downloadSegmentWithRetry 帶重試的下載
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
		Debug("Segment 下載失敗，重試中", "attempt", i+1, "error", err)
	}
	return nil, lastErr
}
