# 信息安全与防御性编码规范 (Security Standard)

> **版本**：v1.0.0  
> **适用范围**：全栈安全策略、鉴权机制、数据加密及 OWASP Top 10 防御。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [api.md](api.md)

---

# 1. 身份认证与授权 (Authentication & Authorization)

## 1.1 JWT (JSON Web Token) 安全规范
1. **算法强约束**：必须使用非对称加密算法（如 `RS256` 或 `ES256`）签发 Token。**严禁允许 `none` 算法或接受弱 HMAC 秘钥**。
2. **Payload 最小化**：JWT Payload 仅存储非敏感字段（如 `user_id`, `role`, `exp`），**绝对禁止在 Payload 中存储密码 Hash、手机号、身份证或支付信息**。
3. **过期机制**：Access Token 有效期必须小于 **2 小时**，结合 Refresh Token 机制（刷新令牌存储在 Redis 中，支持主动撤销）。

## 1.2 越权访问防御 (BOLA / IDOR)
- 所有涉及个体数据的接口（如 `/api/v1/orders/{order_id}`），后端必须进行**显式属主校验**：
  ```go
  // 必须校验当前登录用户 ID 是否等于订单所属 ID
  if order.UserID != currentAuthUser.ID {
      return ErrForbidden // 返回 403
  }
  ```

---

# 2. 数据输入校验与注入防御 (Injection Defense)

## 2.1 零信任输入 (Zero Trust Inputs)
所有外部来源数据（HTTP Header, Query, Form, JSON Body, Webhook, CLI）必须经过严格校验与清洗：
- 使用 `go-playground/validator` 在 DTO 层做结构体校验（如 `binding:"required,email,max=100"`）。
- 文件上传限制：校验文件 Magic Number 字节流，禁止仅凭扩展名判定；限制文件大小最大上限；防止路径穿越（`filepath.Clean`）。

## 2.2 常见注入攻击防御
- **SQL 注入**：完全依赖 ORM 参数化查询，原生 SQL 严禁使用字符串拼接 `fmt.Sprintf("SELECT * FROM ... %s", input)`。
- **Command 注入**：禁止使用 `exec.Command("sh", "-c", input)`。必须采用参数切片方式传参：
  ```go
  // 安全方式
  cmd := exec.CommandContext(ctx, "ls", "-l", targetDir)
  ```
- **SSRF (服务端请求伪造)**：外部 HTTP 请求必须校验目标 URL IP 范围，**禁用内网 IP 网段**（如 `127.0.0.1`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`）与 Cloud Metadata IP (`169.254.169.254`)。

---

# 3. 敏感数据脱敏与加密 (Data Masking & Encryption)

## 3.1 凭证与 Secret 零泄漏
1. **禁止硬编码**：源代码中**绝对禁止出现**任何明文 Password、Database Credentials、AWS/阿里云 AK/SK、JWT Key。
2. **配置注入**：Secret 必须通过环境变量或安全配置中心（如 HashiCorp Vault、K8s Secrets）注入。
3. **Git 提交保护**：必须在 Git Hooks 中集成 `gitleaks` 或 `trufflehog` 自动拦截硬编码 Secret 的提交。

## 3.2 敏感数据脱敏 (Data Masking)
在输出日志、日志告警或 API 返回接口中，必须对 PII（个人身份信息）自动脱敏：
- **手机号**：`138****5678`
- **身份证号**：`4401**********1234`
- **银行卡号**：`6222***********8888`
- **日志脱敏 Logger**：自定义 Log Marshaler 实现对敏感字段自动掩码处理。

---

# 4. 限流与 API 防刷 (Rate Limiting)

1. **多级限流机制**：
   - **IP 级限流**：针对未授权接口（如 `/login`, `/register`），单 IP 限制每分钟最多 10 次请求。
   - **用户级限流**：针对已授权 API，限制单用户每秒 QPS。
2. **推荐算法**：使用 Redis + Lua 脚本实现 **令牌桶 (Token Bucket)** 或 **滑动窗口 (Sliding Window)** 算法。

---

# 5. HTTP 安全 Headers 策略

所有外网暴露的 HTTP API 必须配置以下安全响应头：

```text
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Content-Security-Policy: default-src 'self'
Strict-Transport-Security: max-age=31536000; includeSubDomains
```
