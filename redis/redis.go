package redis

import (
	"context"
	"fmt"
	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/logger"
	"strings"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	ModeStandalone = "standalone"
	ModeSentinel   = "sentinel"

	// unreachablePlaceholderAddr 是配置解析失败时使用的占位地址。
	// 选一个必然连不上的端口（:0）而不是 127.0.0.1:6379，
	// 是为了让误用「大声失败」，而不是静默连上开发者本机的 Redis。
	unreachablePlaceholderAddr = "127.0.0.1:0"
)

// RedisClient Redis 客户端
type RedisClient struct {
	Client  *redis.Client
	Context context.Context

	// ConfigErr 在配置解析/校验失败时非 nil。
	//
	// 此时 Client 是一个不可达的占位客户端，任何操作都会返回连接错误。
	// 调用方应检查该字段并中止启动，而不是继续使用。
	ConfigErr error

	redisCfg *config.Redis
}

// NewRedisClient 创建 Redis 客户端实例
// 参数:
//   - redisCfg: Redis 配置
//   - logger: 日志实例
//
// 返回:
//   - *RedisClient: Redis 客户端实例
func NewRedisClient(redisCfg *config.Redis, logger *logger.Logger) *RedisClient {
	return NewRedisClientWithContext(context.Background(), redisCfg, logger)
}

var (
	once  sync.Once
	Redis *RedisClient
)

// ConnectRedisFromConfig 提供一个兼容老代码的全局单例连接方式
// 功能描述:
//   - 对于已经历史遗留使用全局变量 `redis.Redis` 的代码，可以继续使用这个方法。
//   - 更推荐的做法是：在项目中使用 Wire 进行依赖注入，让 `*RedisClient` 作为依赖传入各个模块，避免全局状态。
func ConnectRedisFromConfig(redisCfg *config.Redis, logger *logger.Logger) {
	once.Do(func() {
		Redis = NewRedisClient(redisCfg, logger)
	})
}

func ConnectRedis(address, username, password string, db int) {
	once.Do(func() {
		Redis = NewClient(address, username, password, db)
	})
}

func NewClient(address, username, password string, db int) *RedisClient {
	rds := &RedisClient{}
	rds.Context = context.Background()

	rds.Client = redis.NewClient(&redis.Options{
		Addr:     address,
		Username: username,
		Password: password,
		DB:       db,
	})

	err := rds.Ping()
	logger.LogIf(err)

	return rds
}

// NewRedisClientWithContext 创建 Redis 客户端实例（支持注入 context）
// 功能描述:
//   - 该方法是可复用封装的核心入口，支持 standalone 和 sentinel 两种模式。
//   - Context 主要用于：超时控制、取消、链路追踪（Trace ID 注入）等。
//
// 参数说明:
//   - ctx: context.Context，建议在应用启动阶段传入 context.Background()；在请求级别可使用 WithTimeout/WithCancel 派生。
//   - redisCfg: *config.Redis，Redis 配置（支持 standalone/sentinel）。
//   - logger: *logger.Logger，用于记录连接失败等关键日志。
//
// 返回值:
//   - *RedisClient: Redis 客户端封装，内部持有 go-redis/v9 的 *redis.Client。
func NewRedisClientWithContext(ctx context.Context, redisCfg *config.Redis, logger *logger.Logger) *RedisClient {
	rds := &RedisClient{
		Context:  ctx,
		redisCfg: redisCfg,
	}

	client, err := newGoRedisClient(redisCfg)
	if err != nil {
		logger.Error("Redis 配置错误", zap.Error(err))
		// 不再静默回落到本机 Redis——那会把配置错误掩盖成「连上了但数据不对」。
		// 改为记录 ConfigErr + 不可达占位客户端，让调用方显式发现并处理。
		rds.ConfigErr = err
		rds.Client = redis.NewClient(&redis.Options{Addr: unreachablePlaceholderAddr})
		return rds
	}
	rds.Client = client

	if err := rds.Ping(); err != nil {
		logger.Error("Redis 连接失败", zap.Error(err))
	}
	return rds
}

func newGoRedisClient(redisCfg *config.Redis) (*redis.Client, error) {
	mode := strings.ToLower(strings.TrimSpace(redisCfg.Mode))
	if mode == "" {
		mode = ModeStandalone
	}

	dialTimeout := parseDuration(redisCfg.DialTimeout, 5*time.Second)
	readTimeout := parseDuration(redisCfg.ReadTimeout, 3*time.Second)
	writeTimeout := parseDuration(redisCfg.WriteTimeout, 3*time.Second)
	minRetryBackoff := parseDuration(redisCfg.MinRetryBackoff, 8*time.Millisecond)
	maxRetryBackoff := parseDuration(redisCfg.MaxRetryBackoff, 512*time.Millisecond)

	if mode == ModeSentinel || (redisCfg.SentinelMasterName != "" && len(redisCfg.SentinelAddrs) > 0) {
		if redisCfg.SentinelMasterName == "" {
			return nil, fmt.Errorf("redis sentinel 模式需要配置 sentinel_master_name")
		}
		if len(redisCfg.SentinelAddrs) == 0 {
			return nil, fmt.Errorf("redis sentinel 模式需要配置 sentinel_addrs")
		}

		opts := &redis.FailoverOptions{
			MasterName:    redisCfg.SentinelMasterName,
			SentinelAddrs: redisCfg.SentinelAddrs,
			DB:            redisCfg.DB,

			Username: redisCfg.Username,
			Password: redisCfg.Password,

			SentinelUsername: redisCfg.SentinelUsername,
			SentinelPassword: redisCfg.SentinelPassword,

			DialTimeout:  dialTimeout,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,

			MaxRetries:      defaultInt(redisCfg.MaxRetries, 3),
			MinRetryBackoff: minRetryBackoff,
			MaxRetryBackoff: maxRetryBackoff,
		}

		if redisCfg.PoolSize > 0 {
			opts.PoolSize = redisCfg.PoolSize
		}
		if redisCfg.MinIdleConns > 0 {
			opts.MinIdleConns = redisCfg.MinIdleConns
		}

		return redis.NewFailoverClient(opts), nil
	}

	addr := fmt.Sprintf("%s:%d", redisCfg.Host, redisCfg.Port)
	opts := &redis.Options{
		Addr:     addr,
		Username: redisCfg.Username,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,

		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,

		MaxRetries:      defaultInt(redisCfg.MaxRetries, 3),
		MinRetryBackoff: minRetryBackoff,
		MaxRetryBackoff: maxRetryBackoff,
	}

	if redisCfg.PoolSize > 0 {
		opts.PoolSize = redisCfg.PoolSize
	}
	if redisCfg.MinIdleConns > 0 {
		opts.MinIdleConns = redisCfg.MinIdleConns
	}

	return redis.NewClient(opts), nil
}

func parseDuration(val string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return fallback
	}
	return d
}

func defaultInt(val int, fallback int) int {
	if val <= 0 {
		return fallback
	}
	return val
}

// Ping 用以测试 redis 连接是否正常
func (rds *RedisClient) Ping() error {
	_, err := rds.Client.Ping(rds.Context).Result()
	return err
}

// Set 存储 key 对应的 value，且设置 expiration 过期时间
func (rds *RedisClient) Set(key string, value any, expiration time.Duration) bool {
	if err := rds.Client.Set(rds.Context, key, value, expiration).Err(); err != nil {
		logger.ErrorString("Redis", "Set", err.Error())
		return false
	}
	return true
}

// Get 获取 key 对应的 value
func (rds *RedisClient) Get(key string) string {
	result, err := rds.Client.Get(rds.Context, key).Result()
	if err != nil {
		if err != redis.Nil {
			logger.ErrorString("Redis", "Get", err.Error())
		}
		return ""
	}
	return result
}

// Has 判断一个 key 是否存在，内部错误和 redis.Nil 都返回 false
func (rds *RedisClient) Has(key string) bool {
	count, err := rds.Client.Exists(rds.Context, key).Result()
	if err != nil {
		logger.ErrorString("Redis", "Has", err.Error())
		return false
	}
	return count > 0
}

// Del 删除存储在 redis 里的数据，支持多个 key 传参
func (rds *RedisClient) Del(keys ...string) bool {
	if err := rds.Client.Del(rds.Context, keys...).Err(); err != nil {
		logger.ErrorString("Redis", "Del", err.Error())
		return false
	}
	return true
}

// FlushDB 清空当前 redis db 里的所有数据
func (rds *RedisClient) FlushDB() bool {
	if err := rds.Client.FlushDB(rds.Context).Err(); err != nil {
		logger.ErrorString("Redis", "FlushDB", err.Error())
		return false
	}
	return true
}

// Increment 当参数只有 1 个时，为 key，其值增加 1。
// 当参数有 2 个时，第一个参数为 key ，第二个参数为要增加的值 int64 类型。
func (rds *RedisClient) Increment(parameters ...any) bool {
	switch len(parameters) {
	case 1:
		key, ok := parameters[0].(string)
		if !ok {
			logger.ErrorString("Redis", "Increment", "参数类型错误：key 必须是 string")
			return false
		}
		if err := rds.Client.Incr(rds.Context, key).Err(); err != nil {
			logger.ErrorString("Redis", "Increment", err.Error())
			return false
		}
	case 2:
		key, ok := parameters[0].(string)
		if !ok {
			logger.ErrorString("Redis", "Increment", "参数类型错误：key 必须是 string")
			return false
		}
		value, ok := toInt64(parameters[1])
		if !ok {
			logger.ErrorString("Redis", "Increment", "参数类型错误：value 必须是 int64/int/int32/int16/int8")
			return false
		}
		if err := rds.Client.IncrBy(rds.Context, key, value).Err(); err != nil {
			logger.ErrorString("Redis", "Increment", err.Error())
			return false
		}
	default:
		logger.ErrorString("Redis", "Increment", "参数过多")
		return false
	}
	return true
}

// Decrement 当参数只有 1 个时，为 key，其值减去 1。
// 当参数有 2 个时，第一个参数为 key ，第二个参数为要减去的值 int64 类型。
func (rds *RedisClient) Decrement(parameters ...any) bool {
	switch len(parameters) {
	case 1:
		key, ok := parameters[0].(string)
		if !ok {
			logger.ErrorString("Redis", "Decrement", "参数类型错误：key 必须是 string")
			return false
		}
		if err := rds.Client.Decr(rds.Context, key).Err(); err != nil {
			logger.ErrorString("Redis", "Decrement", err.Error())
			return false
		}
	case 2:
		key, ok := parameters[0].(string)
		if !ok {
			logger.ErrorString("Redis", "Decrement", "参数类型错误：key 必须是 string")
			return false
		}
		value, ok := toInt64(parameters[1])
		if !ok {
			logger.ErrorString("Redis", "Decrement", "参数类型错误：value 必须是 int64/int/int32/int16/int8")
			return false
		}
		if err := rds.Client.DecrBy(rds.Context, key, value).Err(); err != nil {
			logger.ErrorString("Redis", "Decrement", err.Error())
			return false
		}
	default:
		logger.ErrorString("Redis", "Decrement", "参数过多")
		return false
	}
	return true
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int16:
		return int64(n), true
	case int8:
		return int64(n), true
	default:
		return 0, false
	}
}
