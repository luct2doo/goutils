package captcha

import (
	"errors"
	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/redis"
	"time"
)

type RedisStore struct {
	RedisClient *redis.RedisClient
	KeyPrefix   string
	captchaCfg  *config.Captcha
	app         *app.App
}

// NewRedisStore 创建 Redis 存储实例
// 参数:
//   - redisClient: Redis 客户端实例，通过依赖注入获取
//   - captchaCfg: 验证码配置，通过依赖注入获取
//
// 返回:
//   - *RedisStore: Redis 存储实例
func NewRedisStore(redisClient *redis.RedisClient, key string, captchaCfg *config.Captcha, app *app.App) *RedisStore {
	return &RedisStore{
		RedisClient: redisClient,
		KeyPrefix:   key + ":captcha:",
		captchaCfg:  captchaCfg,
		app:         app,
	}
}

// Set 存储验证码
// 参数:
//   - key: 验证码键名
//   - value: 验证码值
//
// 返回:
//   - error: 错误信息
func (s *RedisStore) Set(key, value string) error {
	ExpireTime := time.Minute * time.Duration(s.captchaCfg.ExpireTime)

	if s.app.IsLocal() {
		ExpireTime = time.Minute * time.Duration(s.captchaCfg.DebugExpireTime)
	}

	if ok := s.RedisClient.Set(s.KeyPrefix+key, value, ExpireTime); !ok {
		return errors.New("无法存储图片验证码答案")
	}
	return nil
}

// Get 获取验证码
// 参数:
//   - key: 验证码键名
//   - clear: 是否在获取后删除
//
// 返回:
//   - string: 验证码值
func (s *RedisStore) Get(key string, clear bool) string {
	key = s.KeyPrefix + key
	val := s.RedisClient.Get(key)
	if clear {
		s.RedisClient.Del(key)
	}
	return val
}

// Verify 验证验证码
// 参数:
//   - key: 验证码键名
//   - answer: 用户输入的答案
//   - clear: 是否在验证后删除
//
// 返回:
//   - bool: 验证结果
func (s *RedisStore) Verify(key, answer string, clear bool) bool {
	v := s.Get(key, clear)
	return v == answer
}
