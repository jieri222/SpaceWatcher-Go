package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/jieri222/SpaceWatcher-Go/internal/logger"
	"github.com/jieri222/SpaceWatcher-Go/internal/m3u8"
)

const (
	StateRunning    = "Running"    // 直播中
	StateEnded      = "Ended"      // 已結束
	StateNotStarted = "NotStarted" // 已建立但尚未開始
)

// Observer 監控 Space 狀態
type Observer struct {
	session  *TwitterSession
	interval time.Duration
	retry    int
}

// NewObserver 建立監控器
func NewObserver(session *TwitterSession, interval time.Duration, retry int) *Observer {
	if interval < 10*time.Second {
		interval = 30 * time.Second
	}
	if retry <= 0 {
		retry = DefaultRetry
	}
	return &Observer{
		session:  session,
		interval: interval,
		retry:    retry,
	}
}

// WaitResult 等待結果
type WaitResult struct {
	SpaceID    string
	Metadata   *SpaceMetadata
	FinalState string
	M3U8URL    string
}

// Resolve 統一入口：取得 metadata → 驗證 → 根據狀態分支處理
// 已結束的 Space 直接解析出 m3u8URL；直播中 / 尚未開始的 Space 進入輪詢等待結束
func (o *Observer) Resolve(ctx context.Context, spaceID string) (*WaitResult, error) {
	// 1. 取得 metadata
	metadata, err := o.fetchStatus(spaceID)
	if err != nil {
		return nil, err
	}

	// 2. 驗證 state 不為空
	if metadata.State == "" {
		return nil, fmt.Errorf("Space 不存在或已被刪除: %s", spaceID)
	}

	logger.Info("取得 Space 資訊", "title", metadata.Title, "state", metadata.State)

	// 3. 根據 state 分支處理
	switch metadata.State {
	case StateEnded:
		return o.resolveEnded(ctx, spaceID, metadata)
	case StateRunning, StateNotStarted:
		return o.waitUntilEnded(ctx, spaceID, metadata)
	default:
		return nil, fmt.Errorf("未知的 Space 狀態: %s", metadata.State)
	}
}

// resolveEnded 處理已結束的 Space
func (o *Observer) resolveEnded(ctx context.Context, spaceID string, metadata *SpaceMetadata) (*WaitResult, error) {
	if !metadata.IsSpaceAvailableforReplay {
		return nil, fmt.Errorf("Space 已結束且不支援重播，無法下載")
	}
	if metadata.MediaKey == "" {
		return nil, fmt.Errorf("此 Space 不支援下載 (無 MediaKey)")
	}

	logger.Info("Space 已結束，取得串流 URL...")

	// 取得 dynamic playlist URL（已結束的 Space 回傳的就是最終 m3u8）
	m3u8URL, err := m3u8.GetSourceLocation(ctx, o.session.client, metadata.MediaKey)
	if err != nil {
		return nil, fmt.Errorf("取得串流 URL 失敗: %w", err)
	}

	logger.Debug("取得 media playlist URL", "url", m3u8URL)

	return &WaitResult{
		SpaceID:    spaceID,
		Metadata:   metadata,
		FinalState: StateEnded,
		M3U8URL:    m3u8URL,
	}, nil
}

// waitUntilEnded 等待 Space 直到結束（Running / NotStarted）
func (o *Observer) waitUntilEnded(ctx context.Context, spaceID string, metadata *SpaceMetadata) (*WaitResult, error) {
	result := &WaitResult{SpaceID: spaceID, Metadata: metadata}

	if metadata.State == StateNotStarted {
		logger.Info("Space 尚未開始，將等到開始後再等待結束", "spaceID", spaceID)
	} else {
		logger.Info("Space 進行中，等待結束", "state", metadata.State)
	}

	// 取得 m3u8 URL，需要 MediaKey，如果 NotStarted 可能還沒有
	var masterURL string
	var err error
	if metadata.MediaKey != "" {
		masterURL, err = o.fetchMasterURL(ctx, metadata.MediaKey)
		if err != nil {
			return nil, err
		}
	}

	// 開始輪詢
	logger.Info("開始等待 Space 結束", "spaceID", spaceID, "interval", o.interval)
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()

	consecutiveErrors := 0
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-ticker.C:
			metadata, err := o.fetchStatus(spaceID)
			if err != nil {
				consecutiveErrors++
				logger.Warn("查詢失敗，重試中", "error", err, "attempt", consecutiveErrors, "maxRetry", o.retry)
				if consecutiveErrors >= o.retry {
					return nil, fmt.Errorf("查詢失敗超過重試次數: %w", err)
				}
				continue
			}
			consecutiveErrors = 0 // 成功後重置

			result.Metadata = metadata

			logger.Debug("檢查 Space 狀態", "spaceID", spaceID, "state", metadata.State)

			// 如果之前是 NotStarted，現在有了 MediaKey，就取得 masterURL
			if masterURL == "" && metadata.MediaKey != "" {
				masterURL, err = o.fetchMasterURL(ctx, metadata.MediaKey)
				if err != nil {
					logger.Warn("取得 Master Playlist URL 失敗，下次重試", "error", err)
					continue
				}
			}

			switch metadata.State {
			case StateEnded:
				result.FinalState = StateEnded
				logger.Debug("Space 已結束，解析 master playlist")

				if masterURL == "" {
					return nil, fmt.Errorf("Space 已結束但無法取得 Master Playlist URL")
				}

				m3u8URL, err := m3u8.ResolveMasterPlaylist(ctx, o.session.client, masterURL)
				if err != nil {
					return nil, fmt.Errorf("解析 master playlist 失敗: %w", err)
				}
				result.M3U8URL = m3u8URL
				logger.Debug("取得 media playlist URL", "url", m3u8URL)
				return result, nil
			case StateRunning:
				logger.Info("Space 進行中，繼續等待...")
			case StateNotStarted:
				logger.Info("Space 尚未開始，繼續等待...")
			}
		}
	}
}

// fetchMasterURL 從 MediaKey 取得 dynamic URL 並推導出 Master Playlist URL
func (o *Observer) fetchMasterURL(ctx context.Context, mediaKey string) (string, error) {
	masterURL, err := m3u8.GetMasterPlaylistURL(ctx, o.session.client, mediaKey)
	if err != nil {
		return "", fmt.Errorf("推導出 master playlist URL 失敗: %w", err)
	}
	logger.Debug("推導出 master playlist URL", "url", masterURL)
	return masterURL, nil
}

// fetchStatus 取得目前狀態
func (o *Observer) fetchStatus(spaceID string) (*SpaceMetadata, error) {
	resp, err := o.session.AudioSpaceById(spaceID)
	if err != nil {
		return nil, err
	}
	return &resp.Data.AudioSpace.Metadata, nil
}
