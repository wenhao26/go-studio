package app

// 本文件定义暴露给 Wails 前端的绑定面（contracts §5，v1 全集）。
//
// 为什么需要独立类型：Wails 会把被注册类型的**全部**导出方法暴露给前端。
// App 上还有 Shutdown/Close（内部生命周期）与 task.Emitter 的
// EmitStatus/EmitProgress/EmitLogs（事件出口）——暴露它们意味着前端可以
// 伪造任务事件、触发关停或关闭数据库（AGENTS.md §66/§68：权限面最小化）。
// 因此绑定注册的是这里的薄委托，而不是 App 本身。

import "github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"

// API 是 Wails 绑定对象：仅含 contracts §5 内的方法。
type API struct {
	app *App
}

// NewAPI 构造绑定对象。
func NewAPI(a *App) *API {
	return &API{app: a}
}

// ChooseDirectory 打开系统原生目录选择器（FR-1）。
func (api *API) ChooseDirectory() (string, error) {
	return api.app.ChooseDirectory()
}

// GetSettings 读取扫描配置与最近目录（FR-1/FU-2/FR-13）。
func (api *API) GetSettings() (Settings, error) {
	return api.app.GetSettings()
}

// SaveSettings 保存扫描配置（不接受 recent_dirs 字段）。
func (api *API) SaveSettings(cfg scanner.Config) error {
	return api.app.SaveSettings(cfg)
}

// RemoveRecentDir 从最近目录移除指定项（幂等；contracts §3.1/§5）。
func (api *API) RemoveRecentDir(dir string) error {
	return api.app.RemoveRecentDir(dir)
}

// StartScan 启动扫描任务，返回 task_id（R4：根目录显式传参）。
func (api *API) StartScan(rootPath string, cfg scanner.Config) (int64, error) {
	return api.app.StartScan(rootPath, cfg)
}

// CancelTask 取消指定任务（协作式）。
func (api *API) CancelTask(taskID int64) error {
	return api.app.CancelTask(taskID)
}

// GetActiveTask 查询当前活动任务（null 表示无活动）。
func (api *API) GetActiveTask() (*ActiveTask, error) {
	return api.app.GetActiveTask()
}

// GetRecoveryNotice 获取异常恢复提示（FR-12，null 表示无恢复）。
func (api *API) GetRecoveryNotice() (*RecoveryNotice, error) {
	return api.app.GetRecoveryNotice()
}

// GetScanSummary 返回一次扫描的终态统计（FR-5）。
func (api *API) GetScanSummary(scanID int64) (ScanSummary, error) {
	return api.app.GetScanSummary(scanID)
}

// ListFiles 分页查询文件列表（category="" 表示全部）。
func (api *API) ListFiles(scanID int64, category string, page, pageSize int) (FilePage, error) {
	return api.app.ListFiles(scanID, category, page, pageSize)
}

// GetCategorySummary 按类别聚合文件计数（FR-7）。
func (api *API) GetCategorySummary(scanID int64) ([]CategoryCount, error) {
	return api.app.GetCategorySummary(scanID)
}

// SetFilePlan 更新单文件整理计划（FR-8）。
func (api *API) SetFilePlan(fileID int64, category string, excluded bool) error {
	return api.app.SetFilePlan(fileID, category, excluded)
}

// StartOrganize 启动整理任务（FR-9）。
func (api *API) StartOrganize(scanID int64) (int64, error) {
	return api.app.StartOrganize(scanID)
}

// GetOrganizeReport 返回整理报告（FR-10）。
func (api *API) GetOrganizeReport(taskID int64) (OrganizeReport, error) {
	return api.app.GetOrganizeReport(taskID)
}

// UndoLastOrganize 撤销上一次可撤销的整理（FR-11）。
func (api *API) UndoLastOrganize(scanID int64) (int64, error) {
	return api.app.UndoLastOrganize(scanID)
}
