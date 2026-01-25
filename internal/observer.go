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
	StateTimedOut = "TimedOut"
	StateCanceled = "Canceled"
)

// Observer 監控 Space 狀態
type Observer struct {
	session  *TwitterSession
	interval time.Duration
}

// NewObserver 建立監控器
func NewObserver(session *TwitterSession, interval time.Duration) *Observer {
	if interval < 10*time.Second {
		interval = 30 * time.Second
	}
	return &Observer{
		session:  session,
		interval: interval,
	}
}

// WatchResult 監控結果
type WatchResult struct {
	SpaceID    string
	Metadata   *SpaceMetadata
	FinalState string
	Error      error
}

// WatchUntilEnded 監控 Space 直到結束
// 返回最終的 metadata 和 state
func (o *Observer) WatchUntilEnded(ctx context.Context, spaceID string) (*WatchResult, error) {
	result := &WatchResult{SpaceID: spaceID}

	// 首次獲取狀態
	resp, err := o.session.AudioSpaceById(spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get initial space info: %w", err)
	}

	metadata := &resp.Data.AudioSpace.Metadata
	result.Metadata = metadata

	// 如果已經結束，直接返回
	if metadata.State == StateEnded {
		result.FinalState = StateEnded
		Info("Space 已結束，準備下載", "spaceID", spaceID)
		return result, nil
	}

	// 開始輪詢
	Info("開始監控 Space", "spaceID", spaceID, "state", metadata.State, "interval", o.interval)
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-ticker.C:
			resp, err := o.session.AudioSpaceById(spaceID)
			if err != nil {
				Warn("查詢失敗，重試中", "error", err)
				continue
			}

			metadata = &resp.Data.AudioSpace.Metadata
			result.Metadata = metadata

			Debug("檢查 Space 狀態", "spaceID", spaceID, "state", metadata.State)

			switch metadata.State {
			case StateEnded:
				result.FinalState = StateEnded
				Info("Space 已結束", "spaceID", spaceID)
				return result, nil
			case StateCanceled, StateTimedOut:
				result.FinalState = metadata.State
				return result, fmt.Errorf("space ended with state: %s", metadata.State)
			default:
				// other states (Running), continue loop
			}
		}
	}
}

// CheckOnce 只檢查一次狀態
func (o *Observer) CheckOnce(spaceID string) (*SpaceMetadata, error) {
	resp, err := o.session.AudioSpaceById(spaceID)
	if err != nil {
		return nil, err
	}
	return &resp.Data.AudioSpace.Metadata, nil
}
