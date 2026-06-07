package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type fakeUpstreamSettings struct{ cdpURL string }

func (f *fakeUpstreamSettings) GetUpstreamManagementRuntime(ctx context.Context) UpstreamManagementRuntime {
	return UpstreamManagementRuntime{BrowserCDPURL: f.cdpURL}
}

func TestChromedpBrowserSolver_DisabledWhenNoCDPURL(t *testing.T) {
	s := NewChromedpBrowserSolver(&fakeUpstreamSettings{cdpURL: ""}, t.TempDir())
	if s.Enabled(context.Background()) {
		t.Fatal("未配置 CDP URL 应禁用")
	}
	_, err := s.SolveCFClearance(context.Background(), &UpstreamProvider{ID: 1})
	if !errors.Is(err, ErrBrowserDisabled) {
		t.Fatalf("禁用时应返回 ErrBrowserDisabled,实际: %v", err)
	}
	_, err = s.AutoLogin(context.Background(), &UpstreamProvider{ID: 1})
	if !errors.Is(err, ErrBrowserDisabled) {
		t.Fatalf("禁用时应返回 ErrBrowserDisabled,实际: %v", err)
	}
}

func TestPruneDiagnostics_KeepsLatestFive(t *testing.T) {
	dir := t.TempDir()
	providerDir := filepath.Join(dir, "upstream-diagnostics", "1")
	if err := os.MkdirAll(providerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 7 张按时间命名的截图
	for i := 0; i < 7; i++ {
		name := "2026060800000" + strconv.Itoa(i) + ".png"
		if err := os.WriteFile(filepath.Join(providerDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneDiagnostics(dir, 1, 5); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(providerDir)
	if len(entries) != 5 {
		t.Fatalf("应保留 5 张,实际 %d", len(entries))
	}
	// 留下的是最新 5 张(字典序最大)
	if entries[0].Name() != "20260608000002.png" {
		t.Fatalf("应删除最旧的,剩余最旧为: %s", entries[0].Name())
	}
}
