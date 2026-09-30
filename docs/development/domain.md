# 业务领域实体、模型与状态机规范 (Domain Standard)

> **版本**：v1.0.0  
> **适用范围**：业务领域建模、通用语言（Ubiquitous Language）、状态机定义及全局 ID 规则。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [database.md](database.md), [api.md](api.md)

---

# 1. 领域通用词汇表 (Ubiquitous Language & Glossary)

为了防止 AI Agent 与开发者在变量名、数据库表字段以及 API JSON Tag 命名时出现不一致或语意模糊，本项目统一建立以下领域术语映射：

| 领域术语 (Domain Term) | 标准英文/代码标识 | 标准数据库字段 / Tag | 领域定义与约束 | 禁用的错误命名 |
|---|---|---|---|---|
| 用户 | User / Account | `user_id`, `account_id` | 系统中的注册实体 | `member`, `usr`, `people` |
| 订单 | Order | `order_no`, `order_id` | 用户发起的交易凭证主表 | `trade`, `bill` |
| 广告触点 | Touchpoint / AdTouch | `touch_id`, `ad_id` | 用户点击/曝光广告产生的归因触点记录 | `click_item`, `ad_log` |
| 归因线索 | Lead | `lead_id` | 广告营销转化生成的高价值潜在客户记录 | `clue`, `customer_info` |
| 账户余额 | Balance | `balance` | 用户在系统的可用资金总额（单位：分） | `money`, `amount` |
| 广告系列 | Campaign | `campaign_id` | 广告投放中的最高层级容器 | `ad_group_parent` |

---

# 2. 状态机规范 (State Machine Standard)

所有业务实体的状态变更必须遵循**显式状态机转移**，禁止随意修改状态字段。

## 2.1 订单状态机 (Order State Machine)

### 状态枚举定义
- `StatusPending` (`10`)：待支付（初始状态）
- `StatusPaying` (`15`)：支付中（已调起第三方支付）
- `StatusPaid` (`20`)：已支付（收到支付成功回调）
- `StatusFailed` (`25`)：支付失败
- `StatusCanceled` (`30`)：已取消（用户手动取消或超时未支付）
- `StatusRefunded` (`40`)：已退款

### 转移合法性矩阵
```text
           ┌──────────────┐
           │ StatusPending│ (10)
           └──────┬───────┘
                  │
        ┌─────────┴─────────┐
        ▼                   ▼
┌──────────────┐    ┌──────────────┐
│ StatusPaying │(15)│StatusCanceled│ (30)
└───────┬──────┘    └──────────────┘
        │
  ┌─────┴─────┐
  ▼           ▼
┌──────────┐ ┌──────────┐
│StatusPaid│ │StatusFail│
└────┬─────┘ └──────────┘
     │ (20)    (25)
     ▼
┌──────────┐
│Refunded  │ (40)
└──────────┘
```

### Go 代码控制规范
```go
var allowedTransitions = map[int8][]int8{
    10: {15, 30}, // Pending -> Paying, Canceled
    15: {20, 25}, // Paying -> Paid, Failed
    20: {40},     // Paid -> Refunded
}

func ValidateStateTransition(current, target int8) bool {
    targets, exists := allowedTransitions[current]
    if !exists {
        return false
    }
    for _, t := range targets {
        if t == target {
            return true
        }
    }
    return false
}
```

---

# 3. 分布式唯一 ID 生成规范 (ID Generation Standard)

为了保证主键高可扩展性、可排序性且不泄漏商业隐私（如订单递增数量），项目统一使用 **Snowflake** 或 **UUID v7** 策略。

## 3.1 ID 类型与业务前缀 (ID Prefixes)
针对暴露给外部 API 的业务单号，必须带有明确的**业务前缀**以提高排查效率：

- **订单号**：`ord_` + Snowflake ID（例：`ord_7182938491028391`）
- **支付流水号**：`pay_` + Snowflake ID（例：`pay_7182938491028392`）
- **触点 ID**：`tch_` + UUID v7（例：`tch_018f3b2a-1234-7890-a1b2-c3d4e5f67890`）

---

# 4. 金额与货币精度规范 (Financial & Money Standard)

1. **绝对禁止使用 `float32` / `float64`** 表示交易金额。
2. **币种单位**：内部计算与数据库存储统一采用**整型 (int64) 表示最小货币单位（分/Cents）**。
   - `100` 代表 `1.00 CNY` / `1.00 USD`。
3. **展示层转换**：仅在 API 返回 Response DTO 或前端渲染时，格式化为带小数点的字符串。
