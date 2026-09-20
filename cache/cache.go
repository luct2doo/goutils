package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/logger"
	"github.com/luct2doo/goutils/redis"

	"github.com/spf13/cast"
)

// CacheService 缓存服务
type CacheService struct {
	store Store
}

// ProvideStore 根据配置提供合适的缓存存储实现
// 当前仅支持 Redis 驱动，后续可扩展其他实现（如内存缓存、文件缓存等）
// 参数:
//   - cacheCfg: 缓存配置
//   - redisClient: Redis客户端
//   - appCfg: 应用配置
//
// 返回:
//   - Store: 缓存存储接口实现
func ProvideStore(cacheCfg *config.Cache, redisClient *redis.RedisClient, appCfg *config.App) Store {
	// TODO: 后续可扩展支持更多驱动，如 "memory"、"file" 等
	return NewRedisStore(redisClient, appCfg)
}

// NewCacheService 创建缓存服务实例
// 参数:
//   - store: 缓存存储接口实现
//
// 返回:
//   - *CacheService: 缓存服务实例
func NewCacheService(store Store) *CacheService {
	return &CacheService{
		store: store,
	}
}

// Set 设置缓存，支持任意类型值
// 参数:
//   - key: 缓存键
//   - obj: 要缓存的值（任意类型）
//   - expireTime: 过期时间
func (c *CacheService) Set(ctx context.Context, key string, obj any, expireTime time.Duration) {
	b, err := json.Marshal(&obj)
	logger.LogIf(err)
	c.store.Set(ctx, key, string(b), expireTime)
}

// Get 获取缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - any: 缓存的值（任意类型）
func (c *CacheService) Get(ctx context.Context, key string) any {
	stringValue := c.store.Get(ctx, key)
	var wanted any
	err := json.Unmarshal([]byte(stringValue), &wanted)
	logger.LogIf(err)
	return wanted
}

// Has 判断缓存是否存在
// 参数:
//   - key: 缓存键
//
// 返回:
//   - bool: 是否存在
func (c *CacheService) Has(ctx context.Context, key string) bool {
	return c.store.Has(ctx, key)
}

// GetObject 获取缓存并解析到指定对象
// 应该传地址，用法如下:
// model := user.User{}
// cache.GetObject("key", &model)
// 参数:
//   - key: 缓存键
//   - wanted: 目标对象（传址）
func (c *CacheService) GetObject(ctx context.Context, key string, wanted any) {
	val := c.store.Get(ctx, key)
	if len(val) > 0 {
		err := json.Unmarshal([]byte(val), &wanted)
		logger.LogIf(err)
	}
}

// GetString 获取字符串类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - string: 字符串值
func (c *CacheService) GetString(ctx context.Context, key string) string {
	return cast.ToString(c.Get(ctx, key))
}

// GetBool 获取布尔类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - bool: 布尔值
func (c *CacheService) GetBool(ctx context.Context, key string) bool {
	return cast.ToBool(c.Get(ctx, key))
}

// GetInt 获取整数类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - int: 整数值
func (c *CacheService) GetInt(ctx context.Context, key string) int {
	return cast.ToInt(c.Get(ctx, key))
}

// GetInt32 获取int32类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - int32: int32值
func (c *CacheService) GetInt32(ctx context.Context, key string) int32 {
	return cast.ToInt32(c.Get(ctx, key))
}

// GetInt64 获取int64类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - int64: int64值
func (c *CacheService) GetInt64(ctx context.Context, key string) int64 {
	return cast.ToInt64(c.Get(ctx, key))
}

// GetUint 获取uint类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - uint: uint值
func (c *CacheService) GetUint(ctx context.Context, key string) uint {
	return cast.ToUint(c.Get(ctx, key))
}

// GetUint32 获取uint32类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - uint32: uint32值
func (c *CacheService) GetUint32(ctx context.Context, key string) uint32 {
	return cast.ToUint32(c.Get(ctx, key))
}

// GetUint64 获取uint64类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - uint64: uint64值
func (c *CacheService) GetUint64(ctx context.Context, key string) uint64 {
	return cast.ToUint64(c.Get(ctx, key))
}

// GetFloat64 获取float64类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - float64: float64值
func (c *CacheService) GetFloat64(ctx context.Context, key string) float64 {
	return cast.ToFloat64(c.Get(ctx, key))
}

// GetTime 获取时间类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - time.Time: 时间值
func (c *CacheService) GetTime(ctx context.Context, key string) time.Time {
	return cast.ToTime(c.Get(ctx, key))
}

// GetDuration 获取时间间隔类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - time.Duration: 时间间隔值
func (c *CacheService) GetDuration(ctx context.Context, key string) time.Duration {
	return cast.ToDuration(c.Get(ctx, key))
}

// GetIntSlice 获取整数切片类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - []int: 整数切片
func (c *CacheService) GetIntSlice(ctx context.Context, key string) []int {
	return cast.ToIntSlice(c.Get(ctx, key))
}

// GetStringSlice 获取字符串切片类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - []string: 字符串切片
func (c *CacheService) GetStringSlice(ctx context.Context, key string) []string {
	return cast.ToStringSlice(c.Get(ctx, key))
}

// GetStringMap 获取字符串映射类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - map[string]any: 字符串映射
func (c *CacheService) GetStringMap(ctx context.Context, key string) map[string]any {
	return cast.ToStringMap(c.Get(ctx, key))
}

// GetStringMapString 获取字符串-字符串映射类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - map[string]string: 字符串-字符串映射
func (c *CacheService) GetStringMapString(ctx context.Context, key string) map[string]string {
	return cast.ToStringMapString(c.Get(ctx, key))
}

// GetStringMapStringSlice 获取字符串-字符串切片映射类型的缓存
// 参数:
//   - key: 缓存键
//
// 返回:
//   - map[string][]string: 字符串-字符串切片映射
func (c *CacheService) GetStringMapStringSlice(ctx context.Context, key string) map[string][]string {
	return cast.ToStringMapStringSlice(c.Get(ctx, key))
}

// Forget 删除缓存
// 参数:
//   - key: 缓存键
func (c *CacheService) Forget(ctx context.Context, key string) {
	c.store.Forget(ctx, key)
}

// Forever 永久存储，支持任意类型值（自动 JSON 序列化）
// 参数:
//   - key: 缓存键
//   - value: 缓存值（任意类型）
func (c *CacheService) Forever(ctx context.Context, key string, value any) {
	b, err := json.Marshal(&value)
	logger.LogIf(err)
	c.store.Forever(ctx, key, string(b))
}

// Flush 清空所有缓存
func (c *CacheService) Flush(ctx context.Context) {
	c.store.Flush(ctx)
}

// Increment 自增
// 参数:
//   - parameters: 自增参数
func (c *CacheService) Increment(ctx context.Context, parameters ...any) {
	c.store.Increment(ctx, parameters...)
}

// Decrement 自减
// 参数:
//   - parameters: 自减参数
func (c *CacheService) Decrement(ctx context.Context, parameters ...any) {
	c.store.Decrement(ctx, parameters...)
}

// IsAlive 判断缓存是否可用
// 返回:
//   - error: 错误信息，nil表示可用
func (c *CacheService) IsAlive(ctx context.Context) error {
	return c.store.IsAlive(ctx)
}
