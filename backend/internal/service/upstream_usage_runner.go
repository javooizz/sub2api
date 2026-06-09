package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const usageRunnerTick = time.Minute

// UpstreamUsageRunner 独立采集调度(节奏跟随各 provider refresh_interval_minutes)。
type UpstreamUsageRunner struct {
	collector *UpstreamUsageCollector
	providers UpstreamProviderRepository
	usage     UpstreamUsageRepository
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func NewUpstreamUsageRunner(collector *UpstreamUsageCollector, providers UpstreamProviderRepository, usage UpstreamUsageRepository) *UpstreamUsageRunner {
	return &UpstreamUsageRunner{collector: collector, providers: providers, usage: usage}
}

func (r *UpstreamUsageRunner) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(usageRunnerTick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.tick(ctx)
			}
		}
	}()
	slog.Info("upstream usage: collector runner started")
}

func (r *UpstreamUsageRunner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

// tick 列全部 provider,按 cursor.last_collected_at + interval 判到期后采集。
func (r *UpstreamUsageRunner) tick(ctx context.Context) {
	all, err := r.providers.List(ctx)
	if err != nil {
		slog.Error("upstream usage: 列 provider 失败", "error", err)
		return
	}
	if len(all) == 0 {
		return
	}
	ids := make([]int64, 0, len(all))
	for _, p := range all {
		ids = append(ids, p.ID)
	}
	cursors, _ := r.usage.ListCursors(ctx, ids)
	now := timezone.Now()
	for _, p := range all {
		interval := time.Duration(p.RefreshIntervalMinutes) * time.Minute
		due := true
		if cur := cursors[p.ID]; cur != nil && cur.LastCollectedAt != nil {
			due = cur.LastCollectedAt.Add(interval).Before(now)
		}
		if !due {
			continue
		}
		if err := r.collector.CollectProvider(ctx, p.ID); err != nil {
			slog.Warn("upstream usage: 采集失败", "provider_id", p.ID, "error", err)
		}
	}
}

// ProvideUpstreamUsageCollector wire 组装采集器,并启动采集 runner(进程生命周期内单例)。
// 仿 ProvideUpstreamMonitor:返回 collector(被 handler 依赖),内部 runner.Start()。
func ProvideUpstreamUsageCollector(repo UpstreamUsageRepository, providers UpstreamProviderRepository, fetchers map[string]UpstreamUsageFetcher) *UpstreamUsageCollector {
	collector := NewUpstreamUsageCollector(repo, providers, fetchers)
	runner := NewUpstreamUsageRunner(collector, providers, repo)
	runner.Start()
	return collector
}
