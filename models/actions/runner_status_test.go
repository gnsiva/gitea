// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package actions_test

import (
	"context"
	"testing"
	"time"

	actions_model "code.gitea.io/gitea/models/actions"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
)

func TestRunnerStatusCache(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	runner := &actions_model.ActionRunner{}
	assert.NoError(t, actions_model.CreateRunner(context.Background(), runner))
	// Reload to get ID and default values
	runner = unittest.AssertExistsAndLoadBean(t, &actions_model.ActionRunner{ID: runner.ID})

	// 1. Test GetRunnerStatus with empty cache (should return defaults)
	lastOnline, lastActive := actions_model.GetRunnerStatus(runner.ID, runner.LastOnline, runner.LastActive)
	assert.Equal(t, runner.LastOnline, lastOnline)
	assert.Equal(t, runner.LastActive, lastActive)

	// 2. Test UpdateRunnerStatus
	newLastOnline := timeutil.TimeStampNow()
	newLastActive := timeutil.TimeStampNow()
	actions_model.UpdateRunnerStatus(context.Background(), runner.ID, newLastOnline, newLastActive)

	// 3. Verify GetRunnerStatus returns cached values
	cachedLastOnline, cachedLastActive := actions_model.GetRunnerStatus(runner.ID, runner.LastOnline, runner.LastActive)
	assert.Equal(t, newLastOnline, cachedLastOnline)
	assert.Equal(t, newLastActive, cachedLastActive)

	// 4. Test FlushRunnerStatus
	// 4a. First flush will ALWAYS write because LastUpdateToDB is zero (so it treats it as old/dirty)
	setting.Actions.RunnerStatusFlushInterval = time.Minute
	err := actions_model.FlushRunnerStatus(context.Background(), false)
	assert.NoError(t, err)

	runnerAfterFirstFlush := unittest.AssertExistsAndLoadBean(t, &actions_model.ActionRunner{ID: runner.ID})
	// Should be NEW value
	assert.Equal(t, newLastOnline, runnerAfterFirstFlush.LastOnline)

	// 4b. Update again
	newerLastOnline := timeutil.TimeStampNow().Add(100)
	newerLastActive := timeutil.TimeStampNow().Add(100)
	actions_model.UpdateRunnerStatus(context.Background(), runner.ID, newerLastOnline, newerLastActive)

	// 4c. Flush again (should be Skipped because interval has NOT passed since step 4a)
	err = actions_model.FlushRunnerStatus(context.Background(), false)
	assert.NoError(t, err)

	runnerAfterSkippedFlush := unittest.AssertExistsAndLoadBean(t, &actions_model.ActionRunner{ID: runner.ID})
	// Should be the value from FIRST flush (newLastOnline), not the newer one (newerLastOnline)
	assert.Equal(t, newLastOnline, runnerAfterSkippedFlush.LastOnline)

	// 5. Test FlushRunnerStatus (should write if interval passed)
	setting.Actions.RunnerStatusFlushInterval = 1 * time.Nanosecond
	time.Sleep(1 * time.Millisecond)

	err = actions_model.FlushRunnerStatus(context.Background(), false)
	assert.NoError(t, err)

	runnerAfterFinalFlush := unittest.AssertExistsAndLoadBean(t, &actions_model.ActionRunner{ID: runner.ID})
	// Should be the newest value
	assert.Equal(t, newerLastOnline, runnerAfterFinalFlush.LastOnline)
}

func TestFindRunnersFlush(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	runner := &actions_model.ActionRunner{}
	assert.NoError(t, actions_model.CreateRunner(context.Background(), runner))

	// Set low interval so it WOULD flush if checked, but we want to ignore interval logic for the "force" test if possible,
	// or assume FindRunners uses force.
	// FindRunners uses FlushRunnerStatus(ctx, true) -> ignoreInterval=true.

	// 1. Update status in memory
	newLastOnline := timeutil.TimeStampNow().Add(1000)
	actions_model.UpdateRunnerStatus(context.Background(), runner.ID, newLastOnline, 0)

	// 2. Call FindRunners
	// This should trigger flush.
	// We use IsOnline=true to ensure loop doesn't optimize it away?
	// Actually FindRunners always calls flush.
	runners, _, err := actions_model.FindRunners(context.Background(), actions_model.FindRunnerOptions{
		IDs: []int64{runner.ID},
	})
	assert.NoError(t, err)
	assert.Len(t, runners, 1)
	assert.Equal(t, newLastOnline, runners[0].LastOnline)

	// 3. Verify DB is updated
	runnerInDB := unittest.AssertExistsAndLoadBean(t, &actions_model.ActionRunner{ID: runner.ID})
	assert.Equal(t, newLastOnline, runnerInDB.LastOnline)
}
