package classifier

import "testing"

// TestEngine_Classify 锁定口径 B 的选类 / 置信度 / 原因算法。
//
// 用例覆盖：requirements §5.2 校验示例、requirements-analysis §2.1 全部用例、
// test-and-acceptance §3.1 必测项，以及封顶 / 并列 / 大小写 / 中文 / 无扩展名边界。
func TestEngine_Classify(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		fileName       string
		ext            string
		wantCategory   string
		wantConfidence int
		wantReason     string
	}{
		{
			// 扩展名分（60）高于关键词分（35），但决定类别的是关键词（工作优先级更高）。
			name: "合同.pdf → 工作（语义优先于文档）",
			path: `C:\docs\合同.pdf`, fileName: "合同.pdf", ext: "pdf",
			wantCategory: CategoryWork, wantConfidence: 95, wantReason: "命中关键词: 合同",
		},
		{
			name: "发票2025.pdf → 财务 95%",
			path: `C:\docs\发票2025.pdf`, fileName: "发票2025.pdf", ext: "pdf",
			wantCategory: CategoryFinance, wantConfidence: 95, wantReason: "命中关键词: 发票",
		},
		{
			name: "photo.jpg → 图片 60%（仅扩展名）",
			path: `C:\pics\photo.jpg`, fileName: "photo.jpg", ext: "jpg",
			wantCategory: CategoryImages, wantConfidence: 60, wantReason: "扩展名 .jpg → 图片",
		},
		{
			name: "README（无扩展名、无关键词）→ 其他 0%",
			path: `C:\x\README`, fileName: "README", ext: "",
			wantCategory: CategoryOther, wantConfidence: 0, wantReason: "未命中任何规则",
		},
		{
			// 关键词同类封顶 +70：文档 60 + 工作 70 = 130 → 封顶 100。
			name: "项目方案.docx → 工作 100%（130 截顶）",
			path: `C:\w\项目方案.docx`, fileName: "项目方案.docx", ext: "docx",
			wantCategory: CategoryWork, wantConfidence: 100, wantReason: "命中关键词: 项目",
		},
		{
			// 路径命中（+20）不来自扩展名：文档 60 + 工作 20 = 80。
			name: "D:/合同/a.pdf → 工作 80%（路径命中）",
			path: `D:/合同/a.pdf`, fileName: "a.pdf", ext: "pdf",
			wantCategory: CategoryWork, wantConfidence: 80, wantReason: "路径命中: 合同",
		},
		{
			name: "未列出扩展名且无关键词 → 其他 0%",
			path: `C:\x\data.bin`, fileName: "data.bin", ext: "bin",
			wantCategory: CategoryOther, wantConfidence: 0, wantReason: "未命中任何规则",
		},
		{
			// 大小写：扩展名参数已小写，文件名大写仍应按小写匹配关键词 invoice。
			name: "INVOICE.PDF → 财务（大小写不敏感）",
			path: `C:\x\INVOICE.PDF`, fileName: "INVOICE.PDF", ext: "pdf",
			wantCategory: CategoryFinance, wantConfidence: 95, wantReason: "命中关键词: invoice",
		},
		{
			name: "中文路径含「发票」→ 财务 80%",
			path: `D:\发票\a.txt`, fileName: "a.txt", ext: "txt",
			wantCategory: CategoryFinance, wantConfidence: 80, wantReason: "路径命中: 发票",
		},
		{
			name: "无扩展名但含关键词 → 工作 35%",
			path: `C:\x\项目计划`, fileName: "项目计划", ext: "",
			wantCategory: CategoryWork, wantConfidence: 35, wantReason: "命中关键词: 项目",
		},
		{
			name: "无扩展名且无关键词 concept → 其他 0%",
			path: `C:\x\concept`, fileName: "concept", ext: "",
			wantCategory: CategoryOther, wantConfidence: 0, wantReason: "未命中任何规则",
		},
		{
			// 关键词同类封顶 +70（4×35=140→70）；目录 `C:\f` 无路径规则命中。
			name: "多关键词同类封顶 → 70%（非 140%）",
			path: `C:\f\发票账单工资报销`, fileName: "发票账单工资报销", ext: "",
			wantCategory: CategoryFinance, wantConfidence: 70, wantReason: "命中关键词: 发票",
		},
		{
			// 路径同类封顶 +20（2×20=40→20）：文档 60 + 财务 20 = 80；并列取声明顺序第一（发票）。
			name: "多路径同类封顶 → 80%",
			path: `D:\报销发票\x.txt`, fileName: "x.txt", ext: "txt",
			wantCategory: CategoryFinance, wantConfidence: 80, wantReason: "路径命中: 发票",
		},
		{
			// 同类中关键词分（35）高于路径分（20），原因取关键词。
			name: "同类关键词优先于路径作为原因",
			path: `D:\发票\发票.pdf`, fileName: "发票.pdf", ext: "pdf",
			wantCategory: CategoryFinance, wantConfidence: 100, wantReason: "命中关键词: 发票",
		},
		{
			// 多语义类命中时按优先级（财务 > 工作）：文档 60 + 财务 35 + 工作 35 = 130 → 100。
			name: "财务优先于工作（多语义类命中）",
			path: `C:\x\发票合同.pdf`, fileName: "发票合同.pdf", ext: "pdf",
			wantCategory: CategoryFinance, wantConfidence: 100, wantReason: "命中关键词: 发票",
		},
		{
			// 视频与图片不共存（扩展名唯一），仅校验媒体类回退。
			name: "mv.mkv → 视频 60%",
			path: `C:\v\mv.mkv`, fileName: "mv.mkv", ext: "mkv",
			wantCategory: CategoryVideos, wantConfidence: 60, wantReason: "扩展名 .mkv → 视频",
		},
	}

	e := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Classify(tt.path, tt.fileName, tt.ext)
			if got.Category != tt.wantCategory {
				t.Errorf("Category = %q, want %q", got.Category, tt.wantCategory)
			}
			if got.Confidence != tt.wantConfidence {
				t.Errorf("Confidence = %d, want %d", got.Confidence, tt.wantConfidence)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.Source != SourceRule {
				t.Errorf("Source = %q, want %q", got.Source, SourceRule)
			}
		})
	}
}

// TestEngine_ConfidenceNeverExceedsCap 守住「置信度 ≤ 100」这一硬约束。
func TestEngine_ConfidenceNeverExceedsCap(t *testing.T) {
	// 尽可能多地命中财务/工作/学习关键词与路径，制造高分。
	e := New()
	got := e.Classify(
		`D:\发票\合同\课程\项目文档\电子书\发票账单工资报销合同项目方案报告课程教程笔记.pdf`,
		`发票账单工资报销合同项目方案报告课程教程笔记.pdf`,
		"pdf",
	)
	if got.Confidence > 100 {
		t.Fatalf("Confidence = %d, want <= 100", got.Confidence)
	}
	if got.Confidence != 100 {
		t.Fatalf("Confidence = %d, want 100（该输入必然触发封顶）", got.Confidence)
	}
	if !IsValidCategory(got.Category) {
		t.Fatalf("Category = %q 不是 7 类之一", got.Category)
	}
}

// TestIsValidCategory 校验 7 类判定（FR-8 类别校验的基础）。
func TestIsValidCategory(t *testing.T) {
	for _, c := range Categories {
		if !IsValidCategory(c) {
			t.Errorf("IsValidCategory(%q) = false, want true", c)
		}
	}
	for _, c := range []string{"", "音频", "图片 ", "Images", "其他类"} {
		if IsValidCategory(c) {
			t.Errorf("IsValidCategory(%q) = true, want false", c)
		}
	}
}
