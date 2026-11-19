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
| `paginator` | 分页器（GORM 查询 + 排序字段白名单 + tie-breaker 稳定排序 + OFFSET / keyset 游标两种模式） |
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

### 初始化顺序

`app` 与 `logger` 是其余组件的前置依赖，请最先构造：

```
config → app.NewApp → logger.NewLogger → jwt / redis / database / cache / paginator …
```

- `redis`、`cache`、`limiter`、`paginator`、`response`、`jwt` 内部都会调用 `logger` 的包级函数（`logger.LogIf` 等）。
- 万一漏调 `NewLogger`，包级函数会退化为「写 stderr 的兜底 logger」而**不会 panic**——出现这类 stderr 输出即说明初始化顺序有问题。
- 日志的 `caller` 字段依赖跳帧设置：包级函数 `logger.Info(...)` 与实例方法 `l.Info(...)` 都已校准到真实调用点，改动 logger 初始化时请一并跑 `logger` 包的测试。

### 游标分页（keyset）

`Param.Cursor` 非空即启用游标模式：忽略 `Page` / `Offset`，不执行 `COUNT(*)`，
`Paging` 里的 `CurrentPage` / `TotalPage` / `TotalCount` 均为 0，改用 `NextCursor` + `HasMore` 驱动翻页。
建议给排序键建联合索引，例如 `(created_at DESC, id DESC)`。

```go
cursor, err := paginator.ParseCursor(c.Query("cursor")) // 空串 → nil，自动回到 OFFSET 模式
if err != nil { /* 返回 400 */ }

// 第一个参数必须带 Model，否则 OFFSET 模式下 COUNT(*) 推断不出表名，
// 错误被吞掉后 TotalCount 会静默变成 0（游标模式不 COUNT，不受影响）
paging := p.Paginate(db.Model(&User{}), &users, paginator.Param{
    PerPage:      20,
    Sort:         "created_at",
    Order:        "desc",
    TieBreaker:   "id", // 零值即默认 id，生成 ORDER BY created_at DESC, id DESC
    AllowedSorts: []string{"created_at", "id"},
    Cursor:       cursor,
})
// paging.NextCursor 放进下一次请求的 cursor；paging.HasMore 决定是否继续
```

`Param` 新增了字段，请用 `BuildParam` 或命名字段构造，不要用位置初始化。

`PerPage` 会被钳制在 `maxPerPage = 100`，避免客户端传超大值直接打到数据库。
需要区分「没有数据」和「查询失败」时，用 `PaginateWithError`（返回 `(*Paging, error)`）；
旧的 `Paginate` 保持原签名，查询失败时记录日志并返回空 `*Paging`。

## 安全注意事项

- `paginator.Param.Sort` / `Order` / `TieBreaker` 会被拼进 SQL 的 `ORDER BY` 与游标 `WHERE` 片段。库内已做字形校验（`column` 或 `table.column`）+ 可选白名单（`Param.AllowedSorts`，仅约束 `Sort`），请不要在调用前绕过本库自行拼接。游标里的值一律通过 `?` 绑定；`ParseCursor` 返回的游标字符串来自客户端，不可信，不要用于鉴权或拼接 SQL。
- 游标分页的排序字段若为 NULL 会漏行，请用 NOT NULL 列；时间类型的排序字段建议业务侧自定义 `CursorExtractor`，把值规范成数据库可比较的格式。
- `captcha` 的测试旁路需同时满足 `TestingEnabled=true`、非 production 环境、`TestingKey` 非空且 id 完全匹配。
  `TestingKey` 留空时不会放行任何请求。验证码校验通过后会立即从 Redis 删除（`clear=true`），同一 id 不能重放。
- `cache` 的 key 前缀优先取 `cache.prefix`（`"rugao"` → `rugao:cache:<key>`），留空才回退应用名；所有 Redis 操作都会使用调用方传入的 `context`（`RedisClient.WithContext`），请求超时/取消可以生效。
- `hash.BcryptHash` 失败会返回 `("", err)`，请务必检查 `err`；`BcryptIsHashed` 依据 `$2a$/$2b$/$2y$` 前缀判断，而不是长度。
- `jwt.refresh_threshold` 的单位是**秒**（与其他分钟单位的字段不同），表示距离过期多少秒时返回 `X-Token-Refresh`。
- `rsa` 的 `parsePublicKey` 同时支持 PKIX（`PUBLIC KEY`）与 PKCS#1（`RSA PUBLIC KEY`）两种 PEM。
- `paginator` 的 `PerPage` 上限为 100；`helpers.GenerateSecureDigit(length)` 会真正返回 `length` 位数字，`TrimStruct` 会跳过未导出字段，不再 panic。
- `database.DeleteAllTables`、`redis.FlushDB`、`cache.Flush` 均为破坏性操作，请只在测试环境调用。
- `helpers.GetLocationFromIP` 默认使用 `http://ip-api.com`（明文 HTTP，受免费额度限制）。生产环境请用 `helpers.NewIPAPIProvider` 注入 HTTPS 端点或自建代理。
- `redis.NewRedisClient*` 在配置非法时不会回落到本机 Redis，而是把错误写入 `RedisClient.ConfigErr`，请检查该字段后再使用。
- `limiter.NewLimiter` 在 Redis 不可用时 `store` 为 nil，`CheckRate` 会返回错误而非崩溃；调用方应先判断返回值再继续。

## License

MIT
