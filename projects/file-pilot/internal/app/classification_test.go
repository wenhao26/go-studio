package app

import (
	"context"
	"errors"
	"testing"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/classifier"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

// seedScanWithFiles 在 App 的库里建一条扫描并写入若干文件行，返回 scanID。
func seedScanWithFiles(t *testing.T, a *App, rows []storage.FileRecord) int64 {
	t.Helper()
	ctx := context.Background()
	scanID, err := a.store.CreateScan(ctx, `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}
	if err := a.store.InsertFiles(ctx, scanID, rows); err != nil {
		t.Fatalf("insert files: %v", err)
	}
	return scanID
}

func ruleFile(path, name, ext, category string, confidence int, reason string) storage.FileRecord {
	return storage.FileRecord{
		Path: path, Name: name, Ext: ext, SizeBytes: 1, MtimeMS: 1,
		Category: category, Confidence: confidence, Source: classifier.SourceRule, Reason: reason,
	}
}

// TestGetCategorySummary_FixedSevenOrderZeroFilled：汇总必须固定 7 类顺序且
// 未出现的类别补 0（D-C4），保证前端汇总视图布局稳定。
func TestGetCategorySummary_FixedSevenOrderZeroFilled(t *testing.T) {
	a, _ := newTestApp(t)
	scanID := seedScanWithFiles(t, a, []storage.FileRecord{
		ruleFile(`C:\data\a.jpg`, "a.jpg", "jpg", classifier.CategoryImages, 60, "扩展名 .jpg → 图片"),
		ruleFile(`C:\data\b.jpg`, "b.jpg", "jpg", classifier.CategoryImages, 60, "扩展名 .jpg → 图片"),
		ruleFile(`C:\data\c.pdf`, "c.pdf", "pdf", classifier.CategoryDocs, 60, "扩展名 .pdf → 文档"),
	})

	got, err := a.GetCategorySummary(scanID)
	if err != nil {
		t.Fatalf("GetCategorySummary: %v", err)
	}
	if len(got) != len(classifier.Categories) {
		t.Fatalf("len = %d, want %d", len(got), len(classifier.Categories))
	}
	want := map[string]int64{
		classifier.CategoryImages: 2,
		classifier.CategoryDocs:   1,
	}
	for i, c := range got {
		if c.Category != classifier.Categories[i] {
			t.Fatalf("index %d category = %q, want %q（固定 7 类顺序）", i, c.Category, classifier.Categories[i])
		}
		if c.Count != want[c.Category] {
			t.Errorf("%s count = %d, want %d", c.Category, c.Count, want[c.Category])
		}
	}
}

// TestGetCategorySummary_AgreesWithListFiles：汇总计数与详情分页结果一致（DoD 交叉校验）。
func TestGetCategorySummary_AgreesWithListFiles(t *testing.T) {
	a, _ := newTestApp(t)
	scanID := seedScanWithFiles(t, a, []storage.FileRecord{
		ruleFile(`C:\data\a.jpg`, "a.jpg", "jpg", classifier.CategoryImages, 60, "r"),
		ruleFile(`C:\data\b.pdf`, "b.pdf", "pdf", classifier.CategoryDocs, 60, "r"),
		ruleFile(`C:\data\c.txt`, "c.txt", "txt", classifier.CategoryDocs, 60, "r"),
	})

	for _, c := range classifier.Categories {
		page, err := a.ListFiles(scanID, c, 1, 100)
		if err != nil {
			t.Fatalf("ListFiles(%q): %v", c, err)
		}
		summary, err := a.GetCategorySummary(scanID)
		if err != nil {
			t.Fatalf("GetCategorySummary: %v", err)
		}
		var want int64
		for _, s := range summary {
			if s.Category == c {
				want = s.Count
			}
		}
		if page.TotalCount != want {
			t.Errorf("category %q: 详情 total=%d, 汇总 count=%d", c, page.TotalCount, want)
		}
	}
}

// TestSetFilePlan_ReassignMarksUserManual：改类别后来源=user、置信度=NULL、原因=用户手动指定。
func TestSetFilePlan_ReassignMarksUserManual(t *testing.T) {
	a, _ := newTestApp(t)
	scanID := seedScanWithFiles(t, a, []storage.FileRecord{
		ruleFile(`C:\data\a.txt`, "a.txt", "txt", classifier.CategoryDocs, 60, "扩展名 .txt → 文档"),
	})
	fileID := firstFileID(t, a, scanID)

	if err := a.SetFilePlan(fileID, classifier.CategoryWork, false); err != nil {
		t.Fatalf("SetFilePlan: %v", err)
	}

	page, err := a.ListFiles(scanID, "", 1, 100)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	row := page.Items[0]
	if row.Category != classifier.CategoryWork {
		t.Errorf("category = %q, want %q", row.Category, classifier.CategoryWork)
	}
	if row.Source != classifier.SourceUser {
		t.Errorf("source = %q, want %q", row.Source, classifier.SourceUser)
	}
	if row.Confidence != nil {
		t.Errorf("confidence = %v, want nil（手动）", *row.Confidence)
	}
	if row.Reason != classifier.ReasonManual {
		t.Errorf("reason = %q, want %q", row.Reason, classifier.ReasonManual)
	}
	if row.Excluded {
		t.Error("excluded = true, want false")
	}
}

// TestSetFilePlan_ExcludeKeepsRuleAttributes：仅排除/恢复时只改 excluded，
// 不得把规则文件改写成「用户手动指定」。
func TestSetFilePlan_ExcludeKeepsRuleAttributes(t *testing.T) {
	a, _ := newTestApp(t)
	scanID := seedScanWithFiles(t, a, []storage.FileRecord{
		ruleFile(`C:\data\a.pdf`, "a.pdf", "pdf", classifier.CategoryDocs, 60, "扩展名 .pdf → 文档"),
	})
	fileID := firstFileID(t, a, scanID)

	// 排除：类别参数即当前类别。
	if err := a.SetFilePlan(fileID, classifier.CategoryDocs, true); err != nil {
		t.Fatalf("SetFilePlan exclude: %v", err)
	}
	row := fileRow(t, a, fileID)
	if !row.Excluded {
		t.Fatal("excluded = false, want true")
	}
	if row.Source != classifier.SourceRule || row.Confidence == nil || *row.Confidence != 60 ||
		row.Reason != "扩展名 .pdf → 文档" {
		t.Fatalf("排除不应改写规则属性：%+v", row)
	}

	// 恢复：excluded=0，仍保留规则属性。
	if err := a.SetFilePlan(fileID, classifier.CategoryDocs, false); err != nil {
		t.Fatalf("SetFilePlan restore: %v", err)
	}
	row = fileRow(t, a, fileID)
	if row.Excluded {
		t.Fatal("excluded = true, want false")
	}
	if row.Source != classifier.SourceRule || row.Confidence == nil || *row.Confidence != 60 {
		t.Fatalf("恢复不应改写规则属性：%+v", row)
	}
}

// TestSetFilePlan_InvalidCategory：类别非法返回裸哨兵 ErrCategoryInvalid。
func TestSetFilePlan_InvalidCategory(t *testing.T) {
	a, _ := newTestApp(t)
	scanID := seedScanWithFiles(t, a, []storage.FileRecord{
		ruleFile(`C:\data\a.txt`, "a.txt", "txt", classifier.CategoryDocs, 60, "r"),
	})
	fileID := firstFileID(t, a, scanID)

	err := a.SetFilePlan(fileID, "音频", false)
	if !errors.Is(err, ErrCategoryInvalid) {
		t.Fatalf("err = %v, want ErrCategoryInvalid", err)
	}
	if err.Error() != "invalid category" {
		t.Fatalf("message = %q, want %q（裸哨兵匹配键）", err.Error(), "invalid category")
	}
}

// TestSetFilePlan_FileNotFound：文件不存在返回裸哨兵 ErrFileNotInScan。
func TestSetFilePlan_FileNotFound(t *testing.T) {
	a, _ := newTestApp(t)
	err := a.SetFilePlan(987654, classifier.CategoryOther, false)
	if !errors.Is(err, ErrFileNotInScan) {
		t.Fatalf("err = %v, want ErrFileNotInScan", err)
	}
	if err.Error() != "file not in scan" {
		t.Fatalf("message = %q, want %q（裸哨兵匹配键）", err.Error(), "file not in scan")
	}
}

// firstFileID 取某次扫描第一条文件的 id。
func firstFileID(t *testing.T, a *App, scanID int64) int64 {
	t.Helper()
	page, err := a.ListFiles(scanID, "", 1, 100)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("no files seeded")
	}
	return page.Items[0].ID
}

// fileRow 读取指定文件当前的行（用于断言落库结果）。
func fileRow(t *testing.T, a *App, fileID int64) FileItem {
	t.Helper()
	ctx := context.Background()
	row, err := a.store.GetFilePlan(ctx, fileID)
	if err != nil {
		t.Fatalf("GetFilePlan: %v", err)
	}
	return FileItem{
		ID: row.ID, Category: row.Category, Confidence: row.Confidence,
		Source: row.Source, Reason: row.Reason, Excluded: row.Excluded,
	}
}
