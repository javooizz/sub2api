//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// Model a real database rejecting writes after a job deadline has expired.
type backupContextRepo struct{ *mockSettingRepo }

func (r *backupContextRepo) GetValue(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return r.mockSettingRepo.GetValue(ctx, key)
}

func (r *backupContextRepo) Set(ctx context.Context, key, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.mockSettingRepo.Set(ctx, key, value)
}

func TestBackupService_RecordReadFailureDoesNotOverwriteHistory(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())
	t.Cleanup(svc.bgCancel)
	require.NoError(t, svc.saveRecord(context.Background(), &BackupRecord{ID: "existing", Status: "completed"}))
	original := repo.data[settingKeyBackupRecords]
	repo.getValueErr = errors.New("database temporarily unavailable")
	err := svc.saveRecord(context.Background(), &BackupRecord{ID: "new", Status: "failed"})
	require.ErrorIs(t, err, repo.getValueErr)
	require.Equal(t, original, repo.data[settingKeyBackupRecords])
}

type backupCallbackDumper struct {
	mockDumper
	dump func(context.Context) (io.ReadCloser, error)
}

func (d *backupCallbackDumper) Dump(ctx context.Context) (io.ReadCloser, error) {
	return d.dump(ctx)
}

type backupTTLLockCache struct {
	fakeLeaderLockCache
	ttl time.Duration
}

func (c *backupTTLLockCache) TryAcquireLeaderLock(ctx context.Context, key, owner string, ttl time.Duration) (bool, error) {
	c.ttl = ttl
	return c.fakeLeaderLockCache.TryAcquireLeaderLock(ctx, key, owner, ttl)
}

func TestBackupService_ScheduledDeadlinePersistsFailure(t *testing.T) {
	repo := &backupContextRepo{newMockSettingRepo()}
	seedS3Config(t, repo.mockSettingRepo)
	dumper := &backupCallbackDumper{}
	svc := newTestBackupService(repo.mockSettingRepo, dumper, newMockObjectStore())
	svc.settingRepo = repo
	svc.backupTimeout = 50 * time.Millisecond
	t.Cleanup(svc.bgCancel)
	dumper.dump = func(ctx context.Context) (io.ReadCloser, error) {
		records, err := svc.ListBackups(context.Background())
		require.NoError(t, err)
		require.Len(t, records, 1, "running scheduled job must be visible before pg_dump finishes")
		require.Equal(t, "running", records[0].Status)
		require.Equal(t, "dumping", records[0].Progress)
		<-ctx.Done()
		return nil, errors.New("pg_dump exited with error: signal: killed")
	}

	svc.runScheduledBackup()

	records, err := svc.ListBackups(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "scheduled", records[0].TriggeredBy)
	require.Equal(t, "failed", records[0].Status)
	require.Empty(t, records[0].Progress)
	require.NotEmpty(t, records[0].FinishedAt)
	require.Contains(t, records[0].ErrorMsg, "context deadline exceeded")
	require.Contains(t, records[0].ErrorMsg, "signal: killed")
}

func TestBackupService_ConfiguredTimeoutAppliesToManualAndScheduledJobs(t *testing.T) {
	for _, scheduled := range []bool{false, true} {
		name := "manual"
		if scheduled {
			name = "scheduled"
		}
		t.Run(name, func(t *testing.T) {
			repo := &backupContextRepo{newMockSettingRepo()}
			seedS3Config(t, repo.mockSettingRepo)
			dumper := &backupCallbackDumper{}
			store := newMockObjectStore()
			svc := NewBackupService(repo, &config.Config{
				Backup: config.BackupConfig{TimeoutSeconds: 21600},
			}, &plainEncryptor{}, func(context.Context, *BackupS3Config) (BackupObjectStore, error) {
				return store, nil
			}, dumper)
			t.Cleanup(svc.bgCancel)
			cache := &backupTTLLockCache{}
			svc.SetLeaderLock(cache, nil)
			remaining := make(chan time.Duration, 1)
			dumper.dump = func(ctx context.Context) (io.ReadCloser, error) {
				deadline, ok := ctx.Deadline()
				if ok {
					remaining <- time.Until(deadline)
				} else {
					remaining <- 0
				}
				svc.bgCancel()
				return nil, ctx.Err()
			}

			if scheduled {
				svc.runScheduledBackup()
				require.Greater(t, cache.ttl, 6*time.Hour, "leader lock must cover the entire extended backup")
				require.Empty(t, cache.heldBy(backupScheduledLeaderLockKey))
			} else {
				_, err := svc.StartBackup(context.Background(), "manual", 14)
				require.NoError(t, err)
				svc.wg.Wait()
			}
			left := <-remaining
			require.InDelta(t, float64(6*time.Hour), float64(left), float64(5*time.Second))
			records, err := svc.ListBackups(context.Background())
			require.NoError(t, err)
			require.Len(t, records, 1)
			require.Equal(t, "failed", records[0].Status)
			require.Contains(t, records[0].ErrorMsg, "context canceled")
		})
	}
}
