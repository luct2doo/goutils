package cache

import (
	"context"
	"time"

	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/logger"
	"github.com/luct2doo/goutils/redis"
)

// RedisStore 实现 cache.Store 接口的Redis存储驱动
type RedisStore struct {
	RedisClient *redis.RedisClient
	KeyPrefix   string
}

// NewRedisStore 创建Redis缓存存储实例（前缀由应用名推导，保持历史行为）
// 参数:
//   - redisClient: Redis客户端
//   - appCfg: 应用配置
//
// 返回:
//   - Store: 缓存存储接口实现
func NewRedisStore(redisClient *redis.RedisClient, appCfg *config.App) Store {
	appName := ""
	if appCfg != nil {
		appName = appCfg.Name
	}
	return NewRedisStoreWithPrefix(redisClient, appName+":cache:")
}

// NewRedisStoreWithPrefix 按给定前缀创建缓存存储实例。
// 使用 ProvideStore 时会自动优先采用 config.Cache.Prefix。
func NewRedisStoreWithPrefix(redisClient *redis.RedisClient, prefix string) Store {
	return &RedisStore{
		RedisClient: redisClient,
		KeyPrefix:   prefix,
	}
}

// Set 设置缓存，带过期时间
func (s *RedisStore) Set(ctx context.Context, key, value string, expireTime time.Duration) {
	s.RedisClient.WithContext(ctx).Set(s.KeyPrefix+key, value, expireTime)
}

// Get 获取缓存
func (s *RedisStore) Get(ctx context.Context, key string) string {
	return s.RedisClient.WithContext(ctx).Get(s.KeyPrefix + key)
}

// Has 判断缓存是否存在
func (s *RedisStore) Has(ctx context.Context, key string) bool {
	return s.RedisClient.WithContext(ctx).Has(s.KeyPrefix + key)
}

// Forget 删除缓存
func (s *RedisStore) Forget(ctx context.Context, key string) {
	s.RedisClient.WithContext(ctx).Del(s.KeyPrefix + key)
}

// Forever 永久存储，支持任意类型值
func (s *RedisStore) Forever(ctx context.Context, key string, value any) {
	s.RedisClient.WithContext(ctx).Set(s.KeyPrefix+key, value, 0)
}

// Flush 清空当前应用的所有缓存（只删除带前缀的 key，不影响其他应用数据）
// 使用 SCAN 遍历避免阻塞 Redis，配合批量删除提升效率
func (s *RedisStore) Flush(ctx context.Context) {
	var cursor uint64
	pattern := s.KeyPrefix + "*"
	batchSize := int64(100)
	client := s.RedisClient.WithContext(ctx)

	for {
		keys, nextCursor, err := client.Client.Scan(ctx, cursor, pattern, batchSize).Result()
		if err != nil {
			logger.LogIf(err)
			break
		}

		// 批量删除当前批次的 key
		if len(keys) > 0 {
			client.Client.Del(ctx, keys...)
		}

		// cursor 为 0 表示遍历完成
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
}

// Increment 自增，自动为 key 添加前缀
// 参数:
//   - parameters[0]: key (string)
//   - parameters[1]: 可选，自增的值 (int64/int/int32 等)
func (s *RedisStore) Increment(ctx context.Context, parameters ...any) {
	if len(parameters) >= 1 {
		if key, ok := parameters[0].(string); ok {
			parameters[0] = s.KeyPrefix + key
		}
	}
	s.RedisClient.WithContext(ctx).Increment(parameters...)
}

// Decrement 自减，自动为 key 添加前缀
// 参数:
//   - parameters[0]: key (string)
//   - parameters[1]: 可选，自减的值 (int64/int/int32 等)
func (s *RedisStore) Decrement(ctx context.Context, parameters ...any) {
	if len(parameters) >= 1 {
		if key, ok := parameters[0].(string); ok {
			parameters[0] = s.KeyPrefix + key
		}
	}
	s.RedisClient.WithContext(ctx).Decrement(parameters...)
}

// IsAlive 判断缓存是否可用
func (s *RedisStore) IsAlive(ctx context.Context) error {
	return s.RedisClient.WithContext(ctx).Ping()
}
