package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

// insertTestScan 在给定库里建一个扫描记录供 files 测试使用。
func insertTestScan(t *testing.T, store *Store) int64 {
	t.Helper()
	id, err := store.CreateScan(context.Background(), `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}
	return id
}

func TestInsertFiles_Batch(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	scanID, err := store.CreateScan(ctx, `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}

	rows := []FileRecord{
		{Path: `C:\data\a.jpg`, Name: "a.jpg", Ext: "jpg", SizeBytes: 10, MtimeMS: 2000,
			CtimeMS: 3000, HasCtime: true, Category: "图片", Confidence: 60, Source: "rule", Reason: "扩展名 jpg"},
		{Path: `C:\data\b.pdf`, Name: "b.pdf", Ext: "pdf", SizeBytes: 20, MtimeMS: 2100,
			Category: "文档", Confidence: 60, Source: "rule", Reason: "扩展名 pdf"},
	}
	if err := store.InsertFiles(ctx, scanID, rows); err != nil {
		t.Fatalf("InsertFiles: %v", err)
	}

	items, total, err := store.ListFiles(ctx, scanID, "", 10, 0)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("total=%d len=%d, want 2/2", total, len(items))
	}

	first := items[0]
	if first.Name != "a.jpg" || first.Path != `C:\data\a.jpg` || first.SizeBytes != 10 ||
		first.Category != "图片" || first.Source != "rule" || first.Excluded {
		t.Fatalf("unexpected first row: %+v", first)
	}
	if first.Confidence == nil || *first.Confidence != 60 {
		t.Fatalf("confidence = %v, want 60", first.Confidence)
	}

	// HasCtime=false 的行 ctime 必须是 NULL 而非 0：0 会被误读为 1970。
	var ctime1, ctime2 any
	if err := store.db.QueryRowContext(ctx, `SELECT ctime FROM files WHERE name = 'a.jpg'`).Scan(&ctime1); err != nil {
		t.Fatalf("read ctime a: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT ctime FROM files WHERE name = 'b.pdf'`).Scan(&ctime2); err != nil {
		t.Fatalf("read ctime b: %v", err)
	}
	if ctime1 == nil || fmt.Sprint(ctime1) != "3000" {
		t.Fatalf("a.jpg ctime = %v, want 3000", ctime1)
	}
	if ctime2 != nil {
		t.Fatalf("b.pdf ctime = %v, want NULL", ctime2)
	}
}

// TestInsertFiles_ChunkedBatch 覆盖多行 VALUES 分块路径：25 行 = 两块满行（10+10）
// + 尾部 5 行，要求全部落库且顺序、字段（含 ctime NULL 语义）与输入一致。
func TestInsertFiles_ChunkedBatch(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	scanID := insertTestScan(t, store)

	rows := make([]FileRecord, 0, 25)
	for i := 0; i < 25; i++ {
		rows = append(rows, FileRecord{
			Path: fmt.Sprintf(`C:\data\f%02d.txt`, i), Name: fmt.Sprintf("f%02d.txt", i),
			Ext: "txt", SizeBytes: int64(i), MtimeMS: 1000 + int64(i),
			CtimeMS: 2000 + int64(i), HasCtime: i%2 == 0,
			Category: "文档", Confidence: 60, Source: "rule", Reason: "扩展名 txt",
		})
	}
	if err := store.InsertFiles(ctx, scanID, rows); err != nil {
		t.Fatalf("InsertFiles: %v", err)
	}

	items, total, err := store.ListFiles(ctx, scanID, "", 30, 0)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if total != 25 || len(items) != 25 {
		t.Fatalf("total=%d len=%d, want 25/25", total, len(items))
	}
	for i, it := range items {
		wantName := fmt.Sprintf("f%02d.txt", i)
		if it.Name != wantName || it.SizeBytes != int64(i) || it.Category != "文档" {
			t.Fatalf("items[%d] = %s/%d/%s, want %s/%d/文档", i, it.Name, it.SizeBytes, it.Category, wantName, i)
		}
	}

	var nulls, values int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM files WHERE ctime IS NULL`).Scan(&nulls); err != nil {
		t.Fatalf("count null ctime: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM files WHERE ctime IS NOT NULL`).Scan(&values); err != nil {
		t.Fatalf("count value ctime: %v", err)
	}
	if nulls != 12 || values != 13 {
		t.Fatalf("ctime null=%d value=%d, want 12/13", nulls, values)
	}
}

func TestInsertFiles_EmptyBatchIsNoop(t *testing.T) {
	store := openTestStore(t)
	if err := store.InsertFiles(context.Background(), 1, nil); err != nil {
		t.Fatalf("empty batch: %v", err)
	}
}

func TestInsertFiles_Validation(t *testing.T) {
	store := openTestStore(t)
	if err := store.InsertFiles(context.Background(), 0, []FileRecord{{Name: "x"}}); err == nil {
		t.Error("want error for scan id 0")
	}
}

// TestInsertFiles_ForeignKeyEnforced 证明 foreign_keys=ON 确实生效：
// files.scan_id 指向不存在的扫描必须被拒绝。
func TestInsertFiles_ForeignKeyEnforced(t *testing.T) {
	store := openTestStore(t)
	err := store.InsertFiles(context.Background(), 999, []FileRecord{{Name: "x"}})
	if err == nil {
		t.Fatal("want foreign key error, got nil")
	}
}

func TestListFiles_Pagination(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	scanID, err := store.CreateScan(ctx, `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}
	for i := 0; i < 25; i++ {
		if err := store.InsertFiles(ctx, scanID, []FileRecord{{
			Path: fmt.Sprintf(`C:\data\f%02d.txt`, i),
			Name: fmt.Sprintf("f%02d.txt", i),
			Ext:  "txt", SizeBytes: int64(i), MtimeMS: 1,
			Category: "文档", Confidence: 60, Source: "rule", Reason: "扩展名 txt",
		}}); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	items, total, err := store.ListFiles(ctx, scanID, "", 10, 10)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if total != 25 {
		t.Fatalf("total = %d, want 25", total)
	}
	if len(items) != 10 {
		t.Fatalf("len = %d, want 10", len(items))
	}
	if items[0].ID != 11 || items[0].Name != "f10.txt" {
		t.Fatalf("unexpected first item of page 2: %+v", items[0])
	}

	// 越界页：返回空 items，total 仍准确（前端据此收敛页码）。
	beyond, total, err := store.ListFiles(ctx, scanID, "", 10, 100)
	if err != nil {
		t.Fatalf("ListFiles beyond: %v", err)
	}
	if len(beyond) != 0 || total != 25 {
		t.Fatalf("beyond page: len=%d total=%d, want 0/25", len(beyond), total)
	}
}

func TestListFiles_CategoryFilter(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	scanID := insertTestScan(t, store)
	if err := store.InsertFiles(ctx, scanID, []FileRecord{
		{Path: `C:\data\a.jpg`, Name: "a.jpg", Ext: "jpg", SizeBytes: 1, MtimeMS: 1,
			Category: "图片", Confidence: 60, Source: "rule", Reason: "扩展名"},
		{Path: `C:\data\b.pdf`, Name: "b.pdf", Ext: "pdf", SizeBytes: 1, MtimeMS: 1,
			Category: "文档", Confidence: 60, Source: "rule", Reason: "扩展名"},
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	items, total, err := store.ListFiles(ctx, scanID, "图片", 10, 0)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Category != "图片" {
		t.Fatalf("unexpected filtered result: total=%d items=%+v", total, items)
	}

	// 不存在的类别：空结果而非错误。
	_, total, err = store.ListFiles(ctx, scanID, "视频", 10, 0)
	if err != nil {
		t.Fatalf("ListFiles unknown category: %v", err)
	}
	if total != 0 {
		t.Fatalf("total for unknown category = %d, want 0", total)
	}
}

// TestListFiles_ConfidenceNullIsUserEdited：用户手动调整的行 confidence 为 NULL
// （FR-8），读取时必须表现为 nil 而非 0。
func TestListFiles_ConfidenceNullIsUserEdited(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	scanID := insertTestScan(t, store)
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO files (scan_id, path, name, ext, size_bytes, mtime, category, confidence, source, reason, excluded)
VALUES (?, ?, 'x.txt', 'txt', 5, 1, '其他', NULL, 'user', '用户手动调整', 1)`,
		scanID, `C:\data\x.txt`); err != nil {
		t.Fatalf("insert user row: %v", err)
	}

	items, _, err := store.ListFiles(ctx, scanID, "", 10, 0)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	row := items[0]
	if row.Confidence != nil {
		t.Fatalf("confidence = %v, want nil (user edited)", *row.Confidence)
	}
	if row.Source != "user" || !row.Excluded {
		t.Fatalf("unexpected row: %+v", row)
	}
}

func TestListFiles_Validation(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if _, _, err := store.ListFiles(ctx, 0, "", 10, 0); err == nil {
		t.Error("want error for scan id 0")
	}
	if _, _, err := store.ListFiles(ctx, 1, "", 0, 0); err == nil {
		t.Error("want error for limit < 1")
	}
	if _, _, err := store.ListFiles(ctx, 1, "", 10, -1); err == nil {
		t.Error("want error for negative offset")
	}
}

// TestGetCategoryCounts 校验按类别聚合（FR-7：GROUP BY，只返回出现过的类别）。
func TestGetCategoryCounts(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	scanID := insertTestScan(t, store)

	rows := []FileRecord{
		{Path: `C:\data\a.jpg`, Name: "a.jpg", Ext: "jpg", SizeBytes: 1, MtimeMS: 1,
			Category: "图片", Confidence: 60, Source: "rule", Reason: "扩展名 .jpg → 图片"},
		{Path: `C:\data\b.jpg`, Name: "b.jpg", Ext: "jpg", SizeBytes: 1, MtimeMS: 1,
			Category: "图片", Confidence: 60, Source: "rule", Reason: "扩展名 .jpg → 图片"},
		{Path: `C:\data\c.pdf`, Name: "c.pdf", Ext: "pdf", SizeBytes: 1, MtimeMS: 1,
			Category: "文档", Confidence: 60, Source: "rule", Reason: "扩展名 .pdf → 文档"},
	}
	if err := store.InsertFiles(ctx, scanID, rows); err != nil {
		t.Fatalf("InsertFiles: %v", err)
	}

	counts, err := store.GetCategoryCounts(ctx, scanID)
	if err != nil {
		t.Fatalf("GetCategoryCounts: %v", err)
	}
	if counts["图片"] != 2 || counts["文档"] != 1 {
		t.Fatalf("counts = %v, want 图片=2 文档=1", counts)
	}
	if _, ok := counts["视频"]; ok {
		t.Fatalf("counts must not contain absent categories: %v", counts)
	}

	// 另一次扫描的数据互不串扰。
	other := insertTestScan(t, store)
	counts, err = store.GetCategoryCounts(ctx, other)
	if err != nil {
		t.Fatalf("GetCategoryCounts other: %v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("counts for empty scan = %v, want empty", counts)
	}
}

func TestGetCategoryCounts_Validation(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.GetCategoryCounts(context.Background(), 0); err == nil {
		t.Error("want error for scan id 0")
	}
}

// TestGetFilePlan_NotFound：不存在的文件返回 sql.ErrNoRows（app 层据此上抛 ErrFileNotInScan）。
func TestGetFilePlan_NotFound(t *testing.T) {
	store := openTestStore(t)
	_, err := store.GetFilePlan(context.Background(), 987654)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

// TestUpdateFilePlan_RoundTrip：写入后读回，覆盖 user 标记（confidence NULL）与排除位。
func TestUpdateFilePlan_RoundTrip(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	scanID := insertTestScan(t, store)

	if err := store.InsertFiles(ctx, scanID, []FileRecord{{
		Path: `C:\data\a.txt`, Name: "a.txt", Ext: "txt", SizeBytes: 1, MtimeMS: 1,
		Category: "文档", Confidence: 60, Source: "rule", Reason: "扩展名 .txt → 文档",
	}}); err != nil {
		t.Fatalf("InsertFiles: %v", err)
	}
	items, _, err := store.ListFiles(ctx, scanID, "", 10, 0)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	fileID := items[0].ID

	// 规则来源的初始计划。
	plan, err := store.GetFilePlan(ctx, fileID)
	if err != nil {
		t.Fatalf("GetFilePlan: %v", err)
	}
	if plan.Category != "文档" || plan.Source != "rule" || plan.Confidence == nil || *plan.Confidence != 60 || plan.Excluded {
		t.Fatalf("unexpected initial plan: %+v", plan)
	}

	// 用户改类别：source=user、confidence=NULL、reason=用户手动指定，同时排除。
	if err := store.UpdateFilePlan(ctx, FilePlanUpdate{
		ID: fileID, Category: "工作", Source: "user", Confidence: nil,
		Reason: "用户手动指定", Excluded: true,
	}); err != nil {
		t.Fatalf("UpdateFilePlan: %v", err)
	}

	plan, err = store.GetFilePlan(ctx, fileID)
	if err != nil {
		t.Fatalf("GetFilePlan after update: %v", err)
	}
	if plan.Category != "工作" || plan.Source != "user" || plan.Confidence != nil ||
		plan.Reason != "用户手动指定" || !plan.Excluded {
		t.Fatalf("unexpected updated plan: %+v", plan)
	}

	// 汇总随之变化（即时生效于整理计划）。
	counts, err := store.GetCategoryCounts(ctx, scanID)
	if err != nil {
		t.Fatalf("GetCategoryCounts: %v", err)
	}
	if counts["工作"] != 1 || counts["文档"] != 0 {
		t.Fatalf("counts after update = %v, want 工作=1 文档=0", counts)
	}
}

func TestUpdateFilePlan_NotFound(t *testing.T) {
	store := openTestStore(t)
	err := store.UpdateFilePlan(context.Background(), FilePlanUpdate{
		ID: 987654, Category: "其他", Source: "user", Reason: "用户手动指定",
	})
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestUpdateFilePlan_Validation(t *testing.T) {
	store := openTestStore(t)
	if err := store.UpdateFilePlan(context.Background(), FilePlanUpdate{ID: 0}); err == nil {
		t.Error("want error for file id 0")
	}
}
