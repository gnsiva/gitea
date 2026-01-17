// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package actions

import (
	"context"
	"sync"
	"time"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"
)

type runnerStatus struct {
	LastOnline     timeutil.TimeStamp
	LastActive     timeutil.TimeStamp
	LastUpdateToDB time.Time
}

var (
	runnerStatusCache = make(map[int64]*runnerStatus)
	runnerStatusLock  sync.RWMutex
)

// UpdateRunnerStatus updates runner's status in memory
func UpdateRunnerStatus(ctx context.Context, runnerID int64, lastOnline, lastActive timeutil.TimeStamp) {
	runnerStatusLock.Lock()
	defer runnerStatusLock.Unlock()

	status, ok := runnerStatusCache[runnerID]
	if !ok {
		status = &runnerStatus{}
		runnerStatusCache[runnerID] = status
	}

	if !lastOnline.IsZero() {
		status.LastOnline = lastOnline
	}
	if !lastActive.IsZero() {
		status.LastActive = lastActive
	}
}

// GetRunnerStatus returns the runner's status from memory
// If the runner is not in memory, it returns the provided default values
func GetRunnerStatus(runnerID int64, defaultLastOnline, defaultLastActive timeutil.TimeStamp) (timeutil.TimeStamp, timeutil.TimeStamp) {
	runnerStatusLock.RLock()
	defer runnerStatusLock.RUnlock()

	if status, ok := runnerStatusCache[runnerID]; ok {
		return status.LastOnline, status.LastActive
	}

	return defaultLastOnline, defaultLastActive
}

// FlushRunnerStatus flushes runner status to database
func FlushRunnerStatus(ctx context.Context, ignoreInterval bool) error {
	runnerStatusLock.Lock()
	defer runnerStatusLock.Unlock()

	now := time.Now()
	for runnerID, status := range runnerStatusCache {
		// Check if status has changed since last update
		if !status.LastOnline.AsTime().After(status.LastUpdateToDB) &&
			!status.LastActive.AsTime().After(status.LastUpdateToDB) {
			continue
		}

		if !ignoreInterval && now.Sub(status.LastUpdateToDB) < setting.Actions.RunnerStatusFlushInterval {
			continue
		}

		// Update database
		runner := &ActionRunner{
			ID:         runnerID,
			LastOnline: status.LastOnline,
			LastActive: status.LastActive,
		}

		if err := UpdateRunner(ctx, runner, "last_online", "last_active"); err != nil {
			log.Error("Failed to update runner status to database: %v", err)
			continue
		}

		status.LastUpdateToDB = now
	}

	return nil
}
