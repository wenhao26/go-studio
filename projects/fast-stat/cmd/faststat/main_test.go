package main

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestTerminalWidth(t *testing.T) {
	tests := []struct {
		name string
		cols string
		want int
	}{
		{"valid columns", "100", 100},
		{"unset falls back to default", "", defaultTerminalWidth},
		{"non numeric falls back to default", "wide", defaultTerminalWidth},
		{"zero falls back to default", "0", defaultTerminalWidth},
		{"negative falls back to default", "-5", defaultTerminalWidth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COLUMNS", tt.cols)
			if got := terminalWidth(); got != tt.want {
				t.Errorf("terminalWidth() = %d, want %d (COLUMNS=%q)", got, tt.want, tt.cols)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	level := zap.NewAtomicLevelAt(zap.WarnLevel)

	logger, err := newLogger(level)
	if err != nil {
		t.Fatalf("newLogger 返回错误: %v", err)
	}
	if logger == nil {
		t.Fatal("newLogger 返回 nil logger")
	}
	// 日志写入 stderr，不污染可被管道解析的 stdout 报告。
	if err := logger.Sync(); err != nil {
		// stderr 上的 Sync 在部分平台返回错误，属于已知无害情况。
		t.Logf("logger.Sync 返回（可忽略）: %v", err)
	}
}

// TestIsTerminal_RegularFile 验证普通文件不会被误判为终端。
//
// 判错方向很关键：若普通文件被判成终端，进度行就会被写进重定向的输出文件，
// 污染可被管道解析的报告。
func TestIsTerminal_RegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	defer func() { _ = f.Close() }()

	if isTerminal(f) {
		t.Error("普通文件不应被判定为终端")
	}
}
