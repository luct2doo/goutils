package verifycode

import (
	"time"

	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/redis"
)

type RedisStore struct {
	RedisClient   *redis.RedisClient
	KeyPrefix     string
	verifyCodeCfg *config.VerifyCode
	app           *app.App
}

func NewRedisStore(redisClient *redis.RedisClient, key string, verifyCodeCfg *config.VerifyCode, app *app.App) *RedisStore {
	return &RedisStore{
		RedisClient:   redisClient,
		KeyPrefix:     key,
		verifyCodeCfg: verifyCodeCfg,
		app:           app,
	}
}

func (s *RedisStore) Set(key, value string) bool {
	ExpireTime := time.Minute * time.Duration(s.verifyCodeCfg.ExpireTime)

	if s.app.IsLocal() {
		ExpireTime = time.Minute * time.Duration(s.verifyCodeCfg.DebugExpireTime)
	}

	return s.RedisClient.Set(s.KeyPrefix+key, value, ExpireTime)
}

func (s *RedisStore) Get(key string, clear bool) (value string) {
	key = s.KeyPrefix + key
	val := s.RedisClient.Get(key)

	if clear {
		s.RedisClient.Del(key)
	}

	return val
}

func (s *RedisStore) Verify(key, answer string, clear bool) bool {
	value := s.Get(key, clear)
	return value == answer
}
