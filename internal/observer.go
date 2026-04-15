package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jieri222/SpaceWatcher-Go/internal/logger"
	"github.com/jieri222/SpaceWatcher-Go/internal/m3u8"
)

const (
	StateRunning    = "Running"    // Live stream
	StateEnded      = "Ended"      // Stream ended
	StateNotStarted = "NotStarted" // Scheduled but not yet begun
)

// Observer monitors the status of a Space
type Observer struct {
	session  *TwitterSession
	interval time.Duration
	retry    int
}

// NewObserver creates a new Observer instance
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

// WaitResult encapsulates the final resolution of a Space's wait cycle
type WaitResult struct {
	SpaceID    string
	Metadata   *SpaceMetadata
	FinalState string
	M3U8URL    string
}

// Resolve handles the unified workflow: obtains metadata -> validates -> routes behavior by state.
// Ended Spaces yield the m3u8URL instantly; Running / NotStarted Spaces enter polling routines waiting for an end state.
func (o *Observer) Resolve(ctx context.Context, spaceID string) (*WaitResult, error) {
	// 1. Fetch metadata
	metadata, err := o.fetchStatus(spaceID)
	if err != nil {
		return nil, err
	}

	// 2. Validate state existence
	if metadata.State == "" {
		return nil, fmt.Errorf("space does not exist or has been deleted: %s", spaceID)
	}

	logger.Info("Got Space info", "title", metadata.Title, "state", metadata.State)

	// 3. Branch routing based upon status
	switch metadata.State {
	case StateEnded:
		return o.resolveEnded(ctx, spaceID, metadata)
	case StateRunning, StateNotStarted:
		return o.waitUntilEnded(ctx, spaceID, metadata)
	default:
		return nil, fmt.Errorf("unknown space state: %s", metadata.State)
	}
}

// resolveEnded processes Spaces that have already ended
func (o *Observer) resolveEnded(ctx context.Context, spaceID string, metadata *SpaceMetadata) (*WaitResult, error) {
	if !metadata.IsSpaceAvailableforReplay {
		return nil, fmt.Errorf("space has ended and does not support replay, cannot download")
	}
	if metadata.MediaKey == "" {
		return nil, fmt.Errorf("this space does not support downloading (no MediaKey)")
	}

	logger.Info("Space has ended, getting stream URL...")

	// Request the dynamic playlist URL (finished Spaces return the final direct m3u8)
	m3u8URL, err := m3u8.GetSourceLocation(ctx, o.session.client, metadata.MediaKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve stream URL: %w", err)
	}

	logger.Debug("Got media playlist URL", "url", m3u8URL)

	return &WaitResult{
		SpaceID:    spaceID,
		Metadata:   metadata,
		FinalState: StateEnded,
		M3U8URL:    m3u8URL,
	}, nil
}

// waitUntilEnded polls the Space waiting for its conclusion (handles Running / NotStarted states)
func (o *Observer) waitUntilEnded(ctx context.Context, spaceID string, metadata *SpaceMetadata) (*WaitResult, error) {
	result := &WaitResult{SpaceID: spaceID, Metadata: metadata}

	if metadata.State == StateNotStarted {
		logger.Info("Space has not started yet, will wait for it to start and then wait for it to end")
	} else {
		logger.Info("Space is running, waiting for it to end")
	}

	var masterURL string

	// Begin polling
	logger.Info("Starting to wait for Space to end", "spaceID", spaceID, "interval", o.interval)
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()

	consecutiveErrors := 0
	for {
		// 1. Fetch Master URL if missing (it will retry next time if it fails here)
		if masterURL == "" && metadata.MediaKey != "" {
			var err error
			masterURL, err = o.fetchMasterURL(ctx, metadata.MediaKey)
			if err != nil {
				if errors.Is(err, m3u8.ErrStreamNotFound) {
					// This is expected behavior (especially when NotStarted), change to Debug to avoid spamming
					logger.Debug("Master Playlist URL not ready yet, retrying...")
				} else {
					return nil, fmt.Errorf("failed to get Master Playlist URL: %w", err)
				}
			}
		}

		// 2. If space has ended, perform final processing and return
		if metadata.State == StateEnded {
			result.FinalState = StateEnded
			logger.Debug("Space has ended, parsing master playlist")

			// If no masterURL was captured during the live stream, and the space supports replay, try to fetch it one last time
			if masterURL == "" && metadata.IsSpaceAvailableforReplay {
				var err error
				masterURL, err = o.fetchMasterURL(ctx, metadata.MediaKey)
				if err != nil {
					return nil, fmt.Errorf("space has ended and failed to retrieve replay URL: %w", err)
				}
			}

			if masterURL == "" {
				return nil, fmt.Errorf("space has ended but no master playlist URL was captured (is replay disabled?)")
			}

			m3u8URL, err := m3u8.ResolveMasterPlaylist(ctx, o.session.client, masterURL)
			if err != nil {
				return nil, fmt.Errorf("failed to parse master playlist: %w", err)
			}
			result.M3U8URL = m3u8URL
			logger.Debug("Got media playlist URL", "url", m3u8URL)
			return result, nil
		}

		// 3. Wait for the next tick or context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-ticker.C:
			newMetadata, err := o.fetchStatus(spaceID)
			if err != nil {
				consecutiveErrors++
				logger.Warn("Query failed, retrying", "error", err, "attempt", consecutiveErrors, "maxRetry", o.retry)
				if consecutiveErrors >= o.retry {
					return nil, fmt.Errorf("query failed exceeding retry limit: %w", err)
				}
				continue
			}
			consecutiveErrors = 0
			metadata = newMetadata
			result.Metadata = metadata
			logger.Debug("Checking Space status", "spaceID", spaceID, "state", metadata.State)
		}
	}
}

// fetchMasterURL retrieves the dynamic URL via MediaKey and infers the final Master Playlist URL
func (o *Observer) fetchMasterURL(ctx context.Context, mediaKey string) (string, error) {
	masterURL, err := m3u8.GetMasterPlaylistURL(ctx, o.session.client, mediaKey)
	if err != nil {
		return "", fmt.Errorf("failed to derive master playlist URL: %w", err)
	}
	logger.Debug("Deriving master playlist URL", "url", masterURL)
	return masterURL, nil
}

// fetchStatus retrieves the latest status via API
func (o *Observer) fetchStatus(spaceID string) (*SpaceMetadata, error) {
	resp, err := o.session.AudioSpaceById(spaceID)
	if err != nil {
		return nil, fmt.Errorf("fetch space status %s: %w", spaceID, err)
	}
	return &resp.Data.AudioSpace.Metadata, nil
}
