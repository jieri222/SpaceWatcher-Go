package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/Eyevinn/hls-m3u8/m3u8"
)

const (
	BaseStreamAPI  = "https://api.x.com/1.1/live_video_stream/status/"
	DefaultWorkers = 5
)

// Downloader 下載器
type Downloader struct {
	session     *TwitterSession
	concurrency int
}

// NewDownloader 建立下載器
func NewDownloader(session *TwitterSession, concurrency int) *Downloader {
	if concurrency <= 0 {
		concurrency = DefaultWorkers
	}
	return &Downloader{
		session:     session,
		concurrency: concurrency,
	}
}

// StreamStatus live_video_stream/status 回應
type StreamStatus struct {
	Source struct {
		Location              string `json:"location"`
		NoRedirectPlaybackUrl string `json:"noRedirectPlaybackUrl"`
		Status                string `json:"status"`
		StreamType            string `json:"streamType"`
	} `json:"source"`
	SessionID      string `json:"sessionId"`
	ChatToken      string `json:"chatToken"`
	LifecycleToken string `json:"lifecycleToken"`
	ShareUrl       string `json:"shareUrl"`
}

// GetStreamURL 取得 m3u8 URL
func (d *Downloader) GetStreamURL(ctx context.Context, mediaKey string) (string, error) {
	url := BaseStreamAPI + mediaKey

	resp, err := d.session.client.Get(ctx, url, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("stream status API error %d: %s", resp.StatusCode, string(body))
	}

	var status StreamStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return "", err
	}

	if status.Source.Location == "" {
		return "", fmt.Errorf("no stream location found in response")
	}

	return status.Source.Location, nil
}

// DownloadSpace 完整下載流程 (串流下載+合併)
func (d *Downloader) DownloadSpace(ctx context.Context, mediaKey string, outputPath string) error {
	Info("取得串流 URL...")
	m3u8URL, err := d.GetStreamURL(ctx, mediaKey)
	if err != nil {
		return fmt.Errorf("failed to get stream URL: %w", err)
	}
	Debug("M3U8 URL", "url", m3u8URL)

	// 解析 m3u8
	Info("解析播放清單...")
	segments, baseURL, err := d.parseM3U8(ctx, m3u8URL)
	if err != nil {
		return fmt.Errorf("failed to parse m3u8: %w", err)
	}
	total := len(segments)
	Info("找到 segments", "count", total)

	// 啟動 ffmpeg
	Info("開始下載", "concurrency", d.concurrency)
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-loglevel", "error",
		"-y",           // 覆蓋輸出
		"-i", "pipe:0", // 從 stdin 讀取
		"-c", "copy", // 無損複製
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
	downloadErr := d.streamDownloadAndMerge(ctx, segments, baseURL, stdin, total)

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

	Info("下載完成", "output", outputPath)
	return nil
}

// SegmentResult 下載結果
type SegmentResult struct {
	Index int
	Data  []byte
	Error error
}

// streamDownloadAndMerge 串流下載並按順序寫入 ffmpeg
func (d *Downloader) streamDownloadAndMerge(ctx context.Context, segments []*m3u8.MediaSegment, baseURL string, writer io.Writer, total int) error {
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
					seg := segments[idx]
					segURL := seg.URI
					if !strings.HasPrefix(segURL, "http") {
						segURL = baseURL + segURL
					}

					data, err := d.downloadSegment(ctx, segURL)
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
		for i := range segments {
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

// parseM3U8 解析 m3u8 並返回 segments
func (d *Downloader) parseM3U8(ctx context.Context, m3u8URL string) ([]*m3u8.MediaSegment, string, error) {
	resp, err := d.session.client.Get(ctx, m3u8URL, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	playlist, _, err := m3u8.DecodeFrom(strings.NewReader(string(body)), false)
	if err != nil {
		return nil, "", err
	}

	// 計算 base URL
	baseURL := m3u8URL[:strings.LastIndex(m3u8URL, "/")+1]

	// Media playlist
	media := playlist.(*m3u8.MediaPlaylist)
	segments := media.GetAllSegments()

	return segments, baseURL, nil
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
