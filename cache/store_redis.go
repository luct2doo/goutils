package cache

import (
	"context"
	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/redis"
	"time"
)

// RedisStore 实现 cache.Store 接口的Redis存储驱动
type RedisStore struct {
	RedisClient *redis.RedisClient
	KeyPrefix   string
}

// NewRedisStore 创建Redis缓存存储实例
// 参数:
//   - redisClient: Redis客户端
//   - appCfg: 应用配置
//
// 返回:
//   - Store: 缓存存储接口实现
func NewRedisStore(redisClient *redis.RedisClient, appCfg *config.App) Store {
	return &RedisStore{
		RedisClient: redisClient,
		KeyPrefix:   appCfg.Name + ":cache:",
	}
}

// Set 设置缓存，带过期时间
func (s *RedisStore) Set(ctx context.Context, key, value string, expireTime time.Duration) {
	s.RedisClient.Set(s.KeyPrefix+key, value, expireTime)
}

// Get 获取缓存
func (s *RedisStore) Get(ctx context.Context, key string) string {
	return s.RedisClient.Get(s.KeyPrefix + key)
}

// Has 判断缓存是否存在
func (s *RedisStore) Has(ctx context.Context, key string) bool {
	return s.RedisClient.Has(s.KeyPrefix + key)
}

// Forget 删除缓存
func (s *RedisStore) Forget(ctx context.Context, key string) {
	s.RedisClient.Del(s.KeyPrefix + key)
}

// Forever 永久存储，支持任意类型值
func (s *RedisStore) Forever(ctx context.Context, key string, value any) {
	s.RedisClient.Set(s.KeyPrefix+key, value, 0)
}

// Flush 清空当前应用的所有缓存（只删除带前缀的 key，不影响其他应用数据）
// 使用 SCAN 遍历避免阻塞 Redis，配合批量删除提升效率
func (s *RedisStore) Flush(ctx context.Context) {
	var cursor uint64
	pattern := s.KeyPrefix + "*"
	batchSize := int64(100)

	for {
		keys, nextCursor, err := s.RedisClient.Client.Scan(ctx, cursor, pattern, batchSize).Result()
		if err != nil {
			break
		}

		// 批量删除当前批次的 key
		if len(keys) > 0 {
			s.RedisClient.Client.Del(ctx, keys...)
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
	s.RedisClient.Increment(parameters...)
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
	s.RedisClient.Decrement(parameters...)
}

// IsAlive 判断缓存是否可用
func (s *RedisStore) IsAlive(ctx context.Context) error {
	return s.RedisClient.Ping()
}
