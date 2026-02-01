package internal

import (
	"context"
	"fmt"
	"time"
)

const (
	StateRunning = "Running" // 實際直播中
	StateEnded   = "Ended"
	// not sure the following states existed
	// StateTimedOut = "TimedOut"
	// StateCanceled = "Canceled"
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

// WaitUntilEnded 等待 Space 直到結束
// 返回最終的 metadata 和 state
func (o *Observer) WaitUntilEnded(ctx context.Context, spaceID string) (*WaitResult, error) {
	result := &WaitResult{SpaceID: spaceID}

	// 首次獲取狀態
	metadata, err := o.fetchStatus(spaceID)
	if err != nil {
		return nil, err
	}
	result.Metadata = metadata

	// 先取得 m3u8 URL，避免過程中斷線或是Space沒有存檔
	m3u8URL, err := GetStreamURL(ctx, o.session.client, metadata.MediaKey)
	if err != nil {
		Error("取得 m3u8 URL 失敗", "error", err)
		return nil, err
	}
	Debug("M3U8 URL", "url", m3u8URL)
	result.M3U8URL = m3u8URL

	// 如果已經結束，直接返回
	if metadata.State == StateEnded {
		result.FinalState = StateEnded
		Info("Space 已結束，準備下載", "spaceID", spaceID)
		return result, nil
	}

	// 開始輪詢
	Info("開始等待 Space 結束", "spaceID", spaceID, "interval", o.interval)
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
				Warn("查詢失敗，重試中", "error", err, "attempt", consecutiveErrors, "maxRetry", o.retry)
				if consecutiveErrors >= o.retry {
					return nil, fmt.Errorf("查詢失敗超過重試次數: %w", err)
				}
				continue
			}
			consecutiveErrors = 0 // 成功後重置

			result.Metadata = metadata

			Debug("檢查 Space 狀態", "spaceID", spaceID, "state", metadata.State)

			if metadata.State == StateEnded {
				result.FinalState = StateEnded
				Info("Space 已結束，準備下載", "spaceID", spaceID)
				return result, nil
			}
		}
	}
}

// fetchStatus 取得目前狀態
func (o *Observer) fetchStatus(spaceID string) (*SpaceMetadata, error) {
	resp, err := o.session.AudioSpaceById(spaceID)
	if err != nil {
		return nil, err
	}
	return &resp.Data.AudioSpace.Metadata, nil
}
