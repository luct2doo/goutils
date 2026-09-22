# goutils

`github.com/luct2doo/goutils` —— 通用 Go 工具库，供多个项目复用。

## 设计原则

- **不依赖任何应用侧的 config 包**：库只定义自身需要的配置结构体（见 `config/`），由使用方把配置文件（yaml/env/…）反序列化成这些结构体后注入。
- **不内置配置读取工具**（viper 等）：读取方式由使用方决定。
- **不承载业务语义**：业务字典、业务枚举、库表字段名一律留在业务项目里，不进本库。
- 纯库，无 `main`，无业务逻辑。

## 包一览

| 包 | 说明 |
|---|---|
| `config` | 通用配置结构体（App / Log / JWT / Redis / Database / Cache / Captcha / Paging） |
| `app` | 应用上下文（环境判断、时区、URL 拼接） |
| `logger` | 基于 zap 的日志封装 + GORM 日志适配 |
| `jwt` | JWT 签发 / 解析 / 刷新（gin 中间件友好） |
| `redis` | Redis 客户端封装（standalone / sentinel，含可复用版） |
| `cache` | 缓存服务（`Store` 接口 + Redis 驱动） |
| `database` | GORM 数据库封装 |
| `limiter` | 基于 ulule/limiter 的限流器 |
| `captcha` | 图形验证码（base64Captcha + Redis 存储） |
| `paginator` | 分页器（GORM 查询 + 排序字段白名单校验） |
| `response` | 统一 HTTP 响应封装 |
| `file` | 文件上传 / 头像裁剪（gin + imaging） |
| `helpers` | 通用辅助函数（随机串、时间转换、GORM Updates 构造等） |
| `hash` | 密码哈希（bcrypt）与 MD5 |
| `rsa` | RSA 加解密与签名验签 |
| `coord` | 坐标转换（BD09 → GCJ02） |
| `str` | 字符串相关工具（单复数、命名风格转换） |

## 使用方式

在业务项目的 `go.mod` 中：

```
require github.com/luct2doo/goutils v1.0.0
```

本地开发阶段可用 `replace` 直接指向本地目录（库代码无需任何改动）：

```
replace github.com/luct2doo/goutils => /path/to/goutils
```

也可以用本地文件模块代理按版本号引用（等价于 maven 装到 `~/.m2`）：

```bash
# 把库里当前状态发布成某个版本
./goproxy/publish.sh /path/to/goutils v1.0.0

# 消费方一次性配置
go env -w GOPROXY=file:///path/to/goproxy,https://goproxy.cn,direct
go get github.com/luct2doo/goutils@v1.0.0
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

## 安全注意事项

- `paginator.Param.Sort` / `Order` 会被拼进 SQL 的 `ORDER BY` 片段。库内已做字形校验（`column` 或 `table.column`）+ 可选白名单（`Param.AllowedSorts`），请不要在调用前绕过本库自行拼接。
- `captcha` 的测试旁路需同时满足 `TestingEnabled=true`、非 production 环境、`TestingKey` 非空且 id 完全匹配。生产环境请保持 `TestingEnabled=false`。
- `database.DeleteAllTables`、`redis.FlushDB`、`cache.Flush` 均为破坏性操作，请只在测试环境调用。
- `helpers.GetLocationFromIP` 默认使用 `http://ip-api.com`（明文 HTTP，受免费额度限制）。生产环境请用 `helpers.NewIPAPIProvider` 注入 HTTPS 端点或自建代理。
- `redis.NewRedisClient*` 在配置非法时不会回落到本机 Redis，而是把错误写入 `RedisClient.ConfigErr`，请检查该字段后再使用。

## License

MIT
