package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// filesColumns 是读取列表时的显式列清单（AGENTS.md §22.2：不 SELECT *）。
const filesColumns = `id, path, name, size_bytes, category, confidence, source, reason, excluded`

// insertChunkRows 是单条 INSERT 语句写入的行数。
//
// 实测（196,707 行、500 行/事务、537MB 真实库、synchronous=FULL）：逐行 Exec 约
// 1.91s，每条语句 10 行约 1.64s（-14%）；再增大行数反而退化（20 行 1.8s、50 行
// 1.9s、100 行 2.5s），故取 10。事务边界仍由调用方决定（每 500 行调用一次本方法）。
const insertChunkRows = 10

// fileInsertArgsPerRow 是单行的绑定参数个数（excluded 是共享常量，不占参数）。
const fileInsertArgsPerRow = 11

// insertChunkSQL 返回一条写 n 行的 INSERT：占位符按行展开，excluded 恒为 0
// （扫描写入的行永不排除；用户手动排除走 UpdateFilePlan）。
func insertChunkSQL(n int) string {
	const tuple = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)"
	tuples := make([]string, n)
	for i := range tuples {
		tuples[i] = tuple
	}
	return `INSERT INTO files (scan_id, path, name, ext, size_bytes, mtime, ctime, category, confidence, source, reason, excluded)
VALUES ` + strings.Join(tuples, ", ")
}

// appendFileArgs 按列顺序把 rows 的绑定参数追加到 dst（HasCtime=false 写 NULL）。
func appendFileArgs(dst []any, scanID int64, rows []FileRecord) []any {
	for i := range rows {
		row := &rows[i]
		var ctime any
		if row.HasCtime {
			ctime = row.CtimeMS
		}
		dst = append(dst, scanID, row.Path, row.Name, row.Ext, row.SizeBytes, row.MtimeMS, ctime,
			row.Category, row.Confidence, row.Source, row.Reason)
	}
	return dst
}

// InsertFiles 批量写入 files 行，整体一个事务（requirements §七：每 500 条一个事务）。
//
// 语句粒度是每 insertChunkRows 行一条多行 VALUES，尾部不足一块的行用等长语句一次写完。
// 空批是 no-op；scanID 必须有效（外键引用 scans.id）。
// 本方法是 scanner.FileSink 的结构化实现：签名匹配即实现，无需 import scanner。
func (s *Store) InsertFiles(ctx context.Context, scanID int64, rows []FileRecord) error {
	if len(rows) == 0 {
		return nil
	}
	if scanID <= 0 {
		return fmt.Errorf("insert files: invalid scan id %d", scanID)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin insert files: %w", err)
	}
	// commit 成功后 Rollback 返回 sql.ErrTxDone，属预期路径；见 applyMigration 同款说明。
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, insertChunkSQL(insertChunkRows))
	if err != nil {
		return fmt.Errorf("prepare insert files: %w", err)
	}
	// Stmt 由 tx 管理生命周期；显式 Close 让失败路径尽早释放语句句柄。
	defer stmt.Close()

	// 尾部不足一块的行数单独准备一条等长语句：只有扫描的最后一个批次会走到这里。
	full := len(rows) - len(rows)%insertChunkRows
	var tail *sql.Stmt
	if full < len(rows) {
		tail, err = tx.PrepareContext(ctx, insertChunkSQL(len(rows)-full))
		if err != nil {
			return fmt.Errorf("prepare insert files tail: %w", err)
		}
		defer tail.Close()
	}

	args := make([]any, 0, fileInsertArgsPerRow*insertChunkRows)
	for start := 0; start < full; start += insertChunkRows {
		args = appendFileArgs(args[:0], scanID, rows[start:start+insertChunkRows])
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("insert files starting at row %q: %w", rows[start].Path, err)
		}
	}
	if full < len(rows) {
		args = appendFileArgs(args[:0], scanID, rows[full:])
		if _, err := tail.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("insert files starting at row %q: %w", rows[full].Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert files: %w", err)
	}
	return nil
}

// ListFiles 分页查询某次扫描的文件行；category 为空表示不限类别。
//
// limit/offset 由调用方（API 层）换算，本方法只做防御性校验；
// total 是过滤后的总行数，用于前端分页。
func (s *Store) ListFiles(ctx context.Context, scanID int64, category string, limit, offset int) ([]FileRow, int64, error) {
	if scanID <= 0 {
		return nil, 0, fmt.Errorf("list files: invalid scan id %d", scanID)
	}
	if limit < 1 {
		return nil, 0, fmt.Errorf("list files: invalid limit %d", limit)
	}
	if offset < 0 {
		return nil, 0, fmt.Errorf("list files: invalid offset %d", offset)
	}

	where, args := filesWhere(scanID, category)

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM files`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count files: %w", err)
	}
	if total == 0 {
		return []FileRow{}, 0, nil
	}

	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT `+filesColumns+`
FROM files`+where+`
ORDER BY id
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	out := make([]FileRow, 0, 32)
	for rows.Next() {
		var (
			row      FileRow
			conf     sql.NullInt64
			excluded int64
		)
		if err := rows.Scan(&row.ID, &row.Path, &row.Name, &row.SizeBytes, &row.Category,
			&conf, &row.Source, &row.Reason, &excluded); err != nil {
			return nil, 0, fmt.Errorf("scan file row: %w", err)
		}
		if conf.Valid {
			v := conf.Int64
			row.Confidence = &v
		}
		row.Excluded = excluded != 0
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate file rows: %w", err)
	}
	return out, total, nil
}

// filesWhere 组装过滤条件（参数化，禁止拼接用户输入）。
func filesWhere(scanID int64, category string) (string, []any) {
	if category == "" {
		return ` WHERE scan_id = ?`, []any{scanID}
	}
	return ` WHERE scan_id = ? AND category = ?`, []any{scanID, category}
}

// ListOrganizeCandidates 返回某次扫描中参与整理的文件（excluded=0，FR-9.2）。
//
// 按 category、id 升序：整理计划据此分组并保持稳定顺序。
// 只读计划所需列（AGENTS.md §22.2）；excluded=1 的文件完全不出现在结果中。
func (s *Store) ListOrganizeCandidates(ctx context.Context, scanID int64) ([]OrganizeCandidate, error) {
	if scanID <= 0 {
		return nil, fmt.Errorf("list organize candidates: invalid scan id %d", scanID)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, path, name, size_bytes, category
FROM files
WHERE scan_id = ? AND excluded = 0
ORDER BY category, id`, scanID)
	if err != nil {
		return nil, fmt.Errorf("list organize candidates scan %d: %w", scanID, err)
	}
	defer rows.Close()

	out := make([]OrganizeCandidate, 0, 64)
	for rows.Next() {
		var c OrganizeCandidate
		if err := rows.Scan(&c.ID, &c.Path, &c.Name, &c.SizeBytes, &c.Category); err != nil {
			return nil, fmt.Errorf("scan organize candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organize candidates: %w", err)
	}
	return out, nil
}

// CountOrganizeCandidates 返回某次扫描中参与整理的文件数（excluded=0，FR-9.1）。
//
// 供二次确认展示「即将整理: 文件数量 N」；与 ListOrganizeCandidates 使用同一
// 过滤口径（excluded=0），只做计数不做全量读取。
func (s *Store) CountOrganizeCandidates(ctx context.Context, scanID int64) (int64, error) {
	if scanID <= 0 {
		return 0, fmt.Errorf("count organize candidates: invalid scan id %d", scanID)
	}
	var count int64
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM files
WHERE scan_id = ? AND excluded = 0`, scanID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count organize candidates scan %d: %w", scanID, err)
	}
	return count, nil
}

// GetCategoryCounts 按类别聚合某次扫描的文件计数（FR-7 汇总）。
//
// 走 (scan_id, category, id) 索引，不做内存全量扫描；返回的 map 只含出现过的
// 类别，未出现的类别由调用方按固定 7 类顺序补 0（见 app.GetCategorySummary）。
func (s *Store) GetCategoryCounts(ctx context.Context, scanID int64) (map[string]int64, error) {
	if scanID <= 0 {
		return nil, fmt.Errorf("category counts: invalid scan id %d", scanID)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT category, COUNT(*)
FROM files
WHERE scan_id = ?
GROUP BY category`, scanID)
	if err != nil {
		return nil, fmt.Errorf("query category counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var (
			category string
			count    int64
		)
		if err := rows.Scan(&category, &count); err != nil {
			return nil, fmt.Errorf("scan category count: %w", err)
		}
		counts[category] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate category counts: %w", err)
	}
	return counts, nil
}

// GetFilePlan 读取单文件的整理计划字段；不存在时返回 sql.ErrNoRows。
func (s *Store) GetFilePlan(ctx context.Context, fileID int64) (FilePlanRow, error) {
	var (
		out  FilePlanRow
		conf sql.NullInt64
		excl int64
	)
	row := s.db.QueryRowContext(ctx, `
SELECT id, category, source, confidence, reason, excluded
FROM files
WHERE id = ?`, fileID)
	if err := row.Scan(&out.ID, &out.Category, &out.Source, &conf, &out.Reason, &excl); err != nil {
		if err == sql.ErrNoRows {
			return FilePlanRow{}, err
		}
		return FilePlanRow{}, fmt.Errorf("get file plan %d: %w", fileID, err)
	}
	if conf.Valid {
		v := conf.Int64
		out.Confidence = &v
	}
	out.Excluded = excl != 0
	return out, nil
}

// UpdateFilePlan 写入单文件的整理计划字段（FR-8 即时落库）。
//
// 只更新目标行；行不存在时返回错误。类别判定（是否标记为 user）由调用方
// 依据 GetFilePlan 结果决定，本方法只做数据写入（AGENTS.md §23）。
func (s *Store) UpdateFilePlan(ctx context.Context, p FilePlanUpdate) error {
	if p.ID <= 0 {
		return fmt.Errorf("update file plan: invalid file id %d", p.ID)
	}
	var conf any
	if p.Confidence != nil {
		conf = *p.Confidence
	}
	var excluded int64
	if p.Excluded {
		excluded = 1
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE files
SET category = ?, source = ?, confidence = ?, reason = ?, excluded = ?
WHERE id = ?`,
		p.Category, p.Source, conf, p.Reason, excluded, p.ID)
	if err != nil {
		return fmt.Errorf("update file plan %d: %w", p.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update file plan %d: rows affected: %w", p.ID, err)
	}
	if n == 0 {
		return fmt.Errorf("update file plan %d: file not found", p.ID)
	}
	return nil
}
