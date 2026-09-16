# goutils

`github.com/luct2doo/goutils` —— 通用 Go 工具库，从 [rugao](https://github.com/luct2doo) 项目中抽离而来，供多个项目复用。

## 设计原则

- **不依赖任何应用侧的 config 包**：库只定义自身需要的配置结构体（见 `config/`），由使用方把配置文件（yaml/env/…) 反序列化成这些结构体后注入。
- **不内置配置读取工具**（viper 等）：读取方式由使用方决定。
- 纯库，无 `main`，无业务逻辑。

## 包一览

| 包 | 说明 |
|---|---|
| `config` | 通用配置结构体（App / Log / JWT / Redis / Database 等） |
| `app` | 应用上下文（环境判断、时区、URL 拼接） |
| `logger` | 基于 zap 的日志封装 + GORM 日志适配 |
| `jwt` | JWT 签发 / 解析 / 刷新（gin 中间件友好） |
| `redis` | Redis 客户端封装（standalone / sentinel，含可复用版） |
| `database` | GORM 数据库封装 |
| `limiter` | 基于 ulule/limiter 的限流器 |
| `response` | 统一 HTTP 响应封装 |
| `helpers` | 通用辅助函数 |
| `rsa` | RSA 加解密 |
| `coord` | 坐标相关工具 |
| `str` | 字符串相关工具 |

## 使用方式

在业务项目的 `go.mod` 中：

```
require github.com/luct2doo/goutils v0.1.0
```

本地开发阶段可先用 `replace` 指向本地目录：

```
replace github.com/luct2doo/goutils => ../goutils
```

示例：以 viper 读取配置后注入 logger

```go
import (
    gconfig "github.com/luct2doo/goutils/config"
    "github.com/luct2doo/goutils/app"
    "github.com/luct2doo/goutils/logger"
)

var logCfg gconfig.Log
var appCfg gconfig.App
// ... viper.Unmarshal(&cfg) 后取 cfg.Log / cfg.App

a := app.NewApp(&appCfg)
l := logger.NewLogger(&logCfg, a)
```

## License

MIT
