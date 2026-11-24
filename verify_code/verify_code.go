package verifycode

import (
	"strings"

	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/helpers"
	"github.com/luct2doo/goutils/logger"
	"github.com/luct2doo/goutils/redis"
)

type VerifyCode struct {
	Store
	app           *app.App
	appCfg        *config.App
	verifyCodeCfg *config.VerifyCode
	redis         *redis.RedisClient
}

func NewVerifyCode(app *app.App, appCfg *config.App, verifyCodeCfg *config.VerifyCode, redis *redis.RedisClient) *VerifyCode {
	return &VerifyCode{
		appCfg:        appCfg,
		verifyCodeCfg: verifyCodeCfg,
		app:           app,
		Store: &RedisStore{
			RedisClient: redis,
			KeyPrefix:   appCfg.Name + ":verify_code:",
		},
	}
}

func (vc *VerifyCode) SendSMS(phone string) string {
	return vc.generateVerifyCode(phone)
}

func (vc *VerifyCode) CheckAnswer(key, answer string) bool {
	logger.DebugJSON("验证码", "检查验证码", map[string]string{
		key: answer,
	})

	if !vc.app.IsProduction() && (strings.HasSuffix(key, vc.verifyCodeCfg.DebugEmailSuffix) || strings.HasPrefix(key, vc.verifyCodeCfg.DebugPhonePrefix)) {
		return true
	}

	return vc.Store.Verify(key, answer, false)
}

func (vc *VerifyCode) generateVerifyCode(key string) string {
	code := helpers.RandomNumber(vc.verifyCodeCfg.CodeLength)

	if vc.app.IsLocal() {
		code = vc.verifyCodeCfg.DebugCode
	}

	logger.DebugJSON("验证码", "生成验证码", map[string]string{
		key: code,
	})

	vc.Store.Set(key, code)
	return code
}
