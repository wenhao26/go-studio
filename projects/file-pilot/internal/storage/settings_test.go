package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestSettings_RoundTrip(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if err := store.SetSetting(ctx, "scan_config", `{"workers":4}`); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	// 重复写入是覆盖而非报错（ON CONFLICT DO UPDATE）。
	if err := store.SetSetting(ctx, "scan_config", `{"workers":8}`); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}

	value, err := store.GetSetting(ctx, "scan_config")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if value != `{"workers":8}` {
		t.Fatalf("value = %q, want %q", value, `{"workers":8}`)
	}
}

func TestSettings_MissingKey(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.GetSetting(context.Background(), "absent"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows, got %v", err)
	}
}

func TestSettings_EmptyKeyRejected(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if _, err := store.GetSetting(ctx, ""); err == nil {
		t.Error("GetSetting: want error for empty key")
	}
	if err := store.SetSetting(ctx, "", "v"); err == nil {
		t.Error("SetSetting: want error for empty key")
	}
}
