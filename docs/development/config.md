# 配置与环境变量规范 (Configuration Standard)

> **版本**：v1.0.0  
> **适用范围**：应用配置管理、环境变量注入、环境变量模板及校验规则。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [security.md](security.md)

---

# 1. 核心原则 (Core Principles)

1. **配置与代码彻底分离**：遵守 12-Factor App 原则，禁止将任何环境相关的配置硬编码在源码中。
2. **强类型配置结构体**：配置加载后必须映射为只读的 Go 强类型 Struct，禁止在业务逻辑中直接调用 `os.Getenv("KEY")`。
3. **启动即校验 (Fail-Fast)**：应用启动时必须校验必填配置项。若必填项缺失或格式错误，应用必须终止启动并打印明确的错误日志。

---

# 2. 配置结构体定义与加载 (Config Struct)

使用 `github.com/spf13/viper` 或 `github.com/kelseyhightower/envconfig` 统一管理配置。

## Go 配置结构体示例
```go
package config

import (
    "fmt"
    "time"

    "github.com/go-playground/validator/v10"
    "github.com/spf13/viper"
)

type Config struct {
    App      AppConfig      `mapstructure:"app"`
    Database DatabaseConfig `mapstructure:"database"`
    Redis    RedisConfig    `mapstructure:"redis"`
}

type AppConfig struct {
    Name        string        `mapstructure:"name" validate:"required"`
    Env         string        `mapstructure:"env" validate:"required,oneof=development staging production"`
    Port        int           `mapstructure:"port" validate:"required,min=1024,max=65535"`
    ReadTimeout time.Duration `mapstructure:"read_timeout" validate:"required"`
}

type DatabaseConfig struct {
    Host     string `mapstructure:"host" validate:"required"`
    Port     int    `mapstructure:"port" validate:"required"`
    User     string `mapstructure:"user" validate:"required"`
    Password string `mapstructure:"password" validate:"required"`
    DBName   string `mapstructure:"dbname" validate:"required"`
}

type RedisConfig struct {
    Addr     string `mapstructure:"addr" validate:"required"`
    Password string `mapstructure:"password"`
    DB       int    `mapstructure:"db" validate:"min=0,max=15"`
}

func LoadConfig(path string) (*Config, error) {
    viper.SetConfigFile(path)
    viper.AutomaticEnv() // 自动读取环境变量覆盖配置文件

    if err := viper.ReadInConfig(); err != nil {
        return nil, fmt.Errorf("read config file: %w", err)
    }

    var cfg Config
    if err := viper.Unmarshal(&cfg); err != nil {
        return nil, fmt.Errorf("unmarshal config: %w", err)
    }

    // 强校验
    validate := validator.New()
    if err := validate.Struct(&cfg); err != nil {
        return nil, fmt.Errorf("validate config: %w", err)
    }

    return &cfg, nil
}
```

---

# 3. `.env.example` 环境变量模板规范

项目根目录下必须提供一份非敏感的 `.env.example`，列出系统运行所需的所有环境变量及其默认值示例：

```bash
# -----------------------------------------------------------------------------
# 应用基础配置
# -----------------------------------------------------------------------------
APP_NAME=my_go_service
APP_ENV=development
APP_PORT=8080
APP_READ_TIMEOUT=15s

# -----------------------------------------------------------------------------
# 数据库配置 (MySQL)
# -----------------------------------------------------------------------------
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASSWORD=secret_password_here
DB_NAME=my_database

# -----------------------------------------------------------------------------
# 缓存配置 (Redis)
# -----------------------------------------------------------------------------
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=
REDIS_DB=0
```

---

# 4. 多环境配置管理 (Environment Matrix)

| 环境类型 (Env) | 配置文件来源 | Secret 注入机制 | 日志级别 |
|---|---|---|---|
| `development` | `config.dev.yaml` 或 `.env` | 本地环境变量 | `DEBUG` |
| `staging` | `config.staging.yaml` | K8s Secret / CI/CD 环境变量 | `INFO` |
| `production` | 系统环境变量 / 配置中心 | HashiCorp Vault / AWS Secrets Manager | `WARN` / `ERROR` |
