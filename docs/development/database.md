# 数据库与存储设计规范 (Database Standard)

> **版本**：v1.0.0  
> **适用范围**：MySQL / PostgreSQL Schema 设计、索引规范、ORM 使用及 GORM / Redis 实践。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [architecture.md](architecture.md)

---

# 1. Schema 命名与设计规范 (Schema Design)

## 1.1 表与字段通用规则
1. **表命名**：使用小写字母、下划线分隔的复数名词（如 `users`, `orders`, `order_items`）。
2. **字符集与排序规则**：统一采用 `utf8mb4`，排序规则采用 `utf8mb4_unicode_ci` 或 `utf8mb4_0900_ai_ci`。
3. **引擎**：存储引擎强制使用 `InnoDB`。
4. **必须包含的基础字段**：

```sql
CREATE TABLE `orders` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增主键',
  `order_no` VARCHAR(64) NOT NULL COMMENT '业务唯一订单号',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
  `status` TINYINT NOT NULL DEFAULT '0' COMMENT '状态: 10-待支付, 20-已支付, 30-已取消',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) NULL DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_order_no` (`order_no`),
  KEY `idx_user_id_status` (`user_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单主表';
```

## 1.2 数据类型选择原则
- **主键**：统一采用 `BIGINT UNSIGNED` 扩展性自增主键，或业务分布式唯一 ID（如 Snowflake / UUID v7）。
- **金额**：**绝对禁止使用 `FLOAT` 或 `DOUBLE`**。统一采用 `BIGINT` 表示最小货币单位（如分，1 元 = 100 分），或采用高精度 `DECIMAL(16,4)`。
- **状态与枚举**：采用 `TINYINT`，必须在 SQL 注释（COMMENT）中明确写出枚举值的映射含义。
- **文本**：定长字符串采用 `CHAR`，变长字符串采用 `VARCHAR(N)`。变长字段尽量指定合理的上限，避免无脑 `VARCHAR(255)`。

---

# 2. 索引规范 (Indexing Standards)

1. **命名约定**：
   - 唯一索引：`uk_字段名1_字段名2`
   - 普通索引：`idx_字段名1_字段名2`
2. **最左前缀原则**：建立组合索引时，高频查询字段、高区分度（Cardinality）字段必须放在最左侧。
3. **控制索引数量**：单表索引数量尽量不超过 **5 个**。过多的索引会严重降低写操作（INSERT/UPDATE）性能。
4. **禁止隐式类型转换**：查询条件中的参数类型必须与数据库字段类型匹配（如字段为 `VARCHAR` 类型，查询参数必须传字符串，否则会导致索引失效全表扫描）。

---

# 3. SQL 与 ORM 操作规范 (SQL & ORM Guidelines)

1. **禁止 `SELECT *`**：必须按需要显式指定查询字段，减少网络带宽与序列化开销。
2. **必须参数化**：所有原生 SQL 操作必须使用占位符（`?` 或 `$1`），**绝对禁止直接拼装用户输入字符串**，杜绝 SQL 注入。
3. **N+1 查询问题防护**：在 GORM 等 ORM 中，关联查询必须显式使用 `Preload` 或 `Joins` 预加载，禁止在循环内部依次查询数据库。

```go
// 错误示例：产生 N+1 次数据库查询
var users []model.User
db.Find(&users)
for _, u := range users {
    db.Model(&u).Association("Orders").Find(&u.Orders) // 循环内发 SQL
}

// 正确示例：使用 Preload 一次性预加载
var users []model.User
db.Preload("Orders").Find(&users)
```

4. **大事务防范**：事务范围必须尽可能小。在事务中**严禁调用外部 HTTP API、发送 RPC 请求或进行耗时 CPU 计算**。

---

# 4. Redis 缓存规范 (Redis Standards)

1. **Key 命名规范**：使用冒号分割命名空间，结构为：`系统名:模块名:业务实体:ID`。
   - 示例：`mall:user:profile:10086`
2. **强制带 TTL**：所有写入 Redis 的缓存 Key **必须设置合理的过期时间（TTL）**，禁止无 TTL 长期堆积。
3. **缓存击穿与雪崩防护**：
   - 过期时间添加随机抖动（如 `baseTTL + rand(1-300s)`），防止集中失效。
   - 热点 Key 访问构建分布式锁（如 Redlock）或互斥重建机制。

---

# 5. Schema Migration 规范

1. **DDL 版本化**：所有数据库结构变更必须编写 Migration 文件（使用 `golang-migrate/migrate` 工具）。
2. **文件命名格式**：`YYYYMMDDHHMMSS_description.up.sql` 与 `YYYYMMDDHHMMSS_description.down.sql`。
3. **线上变更限制**：生产环境禁止直接执行破坏性 DDL（如 `DROP COLUMN` 或 `RENAME COLUMN`）。应采用“先加新列 -> 双写 -> 迁移历史数据 -> 废弃旧列”的平滑过渡策略。
