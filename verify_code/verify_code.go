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
	// 必须走 NewRedisStore：RedisStore.Set 依赖 verifyCodeCfg/app 决定 TTL，
	// 直接字面量构造会漏掉这两个字段，第一次发码就 nil pointer panic。
	return &VerifyCode{
		appCfg:        appCfg,
		verifyCodeCfg: verifyCodeCfg,
		app:           app,
		Store: NewRedisStore(
			redis,
			appCfg.Name+":verify_code:",
			verifyCodeCfg,
			app,
		),
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

	// Set 失败（Redis 不可用）时返回空串，让调用方能明确告知用户「发送失败」；
	// 否则码只存在于返回值里，校验时必然失败，用户只会看到「验证码错误」。
	if !vc.Store.Set(key, code) {
		return ""
	}
	return code
}
