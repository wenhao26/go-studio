package app

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/diag"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

// newStore 打开并迁移一个临时库，测试结束自动关闭。
func newStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "file-pilot.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

// newTestApp 构造一个已启动的 App，事件走记录器（Wails 运行时不在场时
// EventsEmit 会 panic，因此默认实现必须被替换）。
func newTestApp(t *testing.T) (*App, *eventRecorder) {
	t.Helper()
	store := newStore(t)
	a := New(store, diag.NewStderr())
	rec := newEventRecorder()
	a.publish = rec.publish
	a.Startup(context.Background())
	t.Cleanup(a.Close)
	return a, rec
}

// eventRecorder 记录全部事件；terminal 缓存终态状态事件供端到端同步。
type eventRecorder struct {
	mu       sync.Mutex
	statuses []statusPayload
	progress []progressPayload
	logs     []logPayload
	terminal chan statusPayload
}

func newEventRecorder() *eventRecorder {
	return &eventRecorder{terminal: make(chan statusPayload, 8)}
}

func (r *eventRecorder) publish(name string, payload any) {
	switch p := payload.(type) {
	case statusPayload:
		r.mu.Lock()
		r.statuses = append(r.statuses, p)
		r.mu.Unlock()
		if p.Status == "completed" || p.Status == "failed" || p.Status == "canceled" {
			select {
			case r.terminal <- p:
			default: // 只同步用，重复终态丢弃即可（容量已远超需求）
			}
		}
	case progressPayload:
		r.mu.Lock()
		r.progress = append(r.progress, p)
		r.mu.Unlock()
	case logPayload:
		r.mu.Lock()
		r.logs = append(r.logs, p)
		r.mu.Unlock()
	default:
		panic("unexpected event payload type: " + name)
	}
}

func (r *eventRecorder) statusList() []statusPayload {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]statusPayload(nil), r.statuses...)
}

func (r *eventRecorder) logTexts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, b := range r.logs {
		for _, e := range b.Entries {
			out = append(out, e.Text)
		}
	}
	return out
}

// containsLog 判断日志文本集合中是否存在包含子串的条目。
func containsLog(texts []string, sub string) bool {
	for _, tx := range texts {
		if strings.Contains(tx, sub) {
			return true
		}
	}
	return false
}

// waitTerminal 等待任务到达终态；超时即失败（安全网，不作为同步手段）。
func waitTerminal(t *testing.T, r *eventRecorder) statusPayload {
	t.Helper()
	select {
	case p := <-r.terminal:
		return p
	case <-time.After(10 * time.Second): // 安全网超时，同步本身靠 terminal 通道
		t.Fatal("task did not reach terminal state in time")
		return statusPayload{}
	}
}
