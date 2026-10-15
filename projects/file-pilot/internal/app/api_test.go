package app

import (
	"reflect"
	"testing"
)

// TestAPI_ExposesOnlyContractMethods 锁定 contracts §5 的绑定方法面：
// Wails 绑定被注册类型的全部导出方法，任何新增都必须先改契约文档
// （本测试同时守护「生命周期/事件出口方法不得暴露给前端」这一安全约束）。
func TestAPI_ExposesOnlyContractMethods(t *testing.T) {
	want := map[string]bool{
		"ChooseDirectory":    false,
		"GetSettings":        false,
		"SaveSettings":       false,
		"RemoveRecentDir":    false,
		"StartScan":          false,
		"CancelTask":         false,
		"GetActiveTask":      false,
		"GetRecoveryNotice":  false,
		"GetScanSummary":     false,
		"ListFiles":          false,
		"GetCategorySummary": false,
		"SetFilePlan":        false,
		"StartOrganize":      false,
		"GetOrganizeReport":  false,
		"UndoLastOrganize":   false,
	}

	rt := reflect.TypeOf((*API)(nil))
	for i := 0; i < rt.NumMethod(); i++ {
		name := rt.Method(i).Name
		if _, ok := want[name]; !ok {
			t.Errorf("API 暴露了契约外的方法 %q（新增绑定必须先更新 contracts §5）", name)
			continue
		}
		want[name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("契约方法 %q 未暴露在 API 上", name)
		}
	}
}
