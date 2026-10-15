-- 0001_init.sql：Phase 1 初始 schema（requirements §七、requirements-analysis §3.1）。
--
-- 约定：
--   * 时间戳一律 INTEGER UTC 毫秒（contracts §1），可空列表示「尚未发生」；
--   * 大小一律 INTEGER 字节；置信度 INTEGER 0–100，用户手动调整时为 NULL（FR-8）；
--   * 索引 (scan_id, category, id) 服务分类计数与分页（§3.2）。

CREATE TABLE scans (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    root_path        TEXT    NOT NULL,
    status           TEXT    NOT NULL,
    started_at       INTEGER NOT NULL,
    finished_at      INTEGER,
    duration_ms      INTEGER,
    file_count       INTEGER NOT NULL DEFAULT 0,
    dir_count        INTEGER NOT NULL DEFAULT 0,
    total_size_bytes INTEGER NOT NULL DEFAULT 0,
    skipped_count    INTEGER NOT NULL DEFAULT 0,
    avg_speed        REAL,
    max_workers      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE files (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id    INTEGER NOT NULL REFERENCES scans(id),
    path       TEXT    NOT NULL,
    name       TEXT    NOT NULL,
    ext        TEXT    NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL,
    mtime      INTEGER NOT NULL,
    ctime      INTEGER,
    category   TEXT    NOT NULL,
    confidence INTEGER,
    source     TEXT    NOT NULL,
    reason     TEXT    NOT NULL,
    excluded   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_files_scan_category_id ON files (scan_id, category, id);

CREATE TABLE tasks (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    type           TEXT    NOT NULL,
    status         TEXT    NOT NULL,
    scan_id        INTEGER REFERENCES scans(id),
    parent_task_id INTEGER,
    started_at     INTEGER,
    finished_at    INTEGER,
    error          TEXT,
    undone         INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_tasks_scan_id ON tasks (scan_id);

CREATE TABLE operations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id    INTEGER NOT NULL REFERENCES tasks(id),
    action     TEXT    NOT NULL,
    src_path   TEXT    NOT NULL,
    dst_path   TEXT    NOT NULL,
    status     TEXT    NOT NULL,
    error      TEXT,
    size_bytes INTEGER,
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_operations_task_action_status ON operations (task_id, action, status);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
