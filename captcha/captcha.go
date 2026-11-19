package captcha

import (
	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/redis"

	"github.com/mojocn/base64Captcha"
)

type CaptchaService struct {
	Base64Captcha *base64Captcha.Captcha
	captchaCfg    *config.Captcha
	appCfg        *config.App
	app           *app.App
}

// NewCaptcha 创建验证码实例
// 参数:
//   - appCfg: 应用配置
//   - captchaCfg: 验证码配置
//   - redisClient: Redis 客户端实例
//
// 返回:
//   - *Captcha: 验证码实例
func NewCaptchaService(appCfg *config.App, captchaCfg *config.Captcha, redisClient *redis.RedisClient, app *app.App) *CaptchaService {
	// 创建 Redis 存储实例
	store := NewRedisStore(
		redisClient,
		appCfg.Name,
		captchaCfg,
		app,
	)

	driver := base64Captcha.NewDriverDigit(
		captchaCfg.Height,
		captchaCfg.Width,
		captchaCfg.Length,
		captchaCfg.MaxSkew,
		captchaCfg.DotCount,
	)

	return &CaptchaService{
		Base64Captcha: base64Captcha.NewCaptcha(driver, store),
		captchaCfg:    captchaCfg,
		appCfg:        appCfg,
		app:           app,
	}
}

func (c *CaptchaService) GenerateCaptcha() (id, b64s, answer string, err error) {
	return c.Base64Captcha.Generate()
}

// VerifyCaptcha 校验图形验证码。
//
// 测试旁路（全局搜索关键字 REMOVE-BEFORE-RELEASE 可统一清理）必须同时满足：
// TestingEnabled=true、非 production 环境、TestingKey 非空、id 完全匹配。
// 四个条件缺一不可——尤其 TestingKey 不能为空，否则「空 id == 空 key」会让所有
// 未携带 captcha_id 的请求直接通过。
func (c *CaptchaService) VerifyCaptcha(id, answer string) (match bool) {
	if c.captchaCfg != nil && c.app != nil &&
		c.captchaCfg.TestingEnabled &&
		!c.app.IsProduction() &&
		c.captchaCfg.TestingKey != "" &&
		id == c.captchaCfg.TestingKey {
		return true
	}
	// clear=true：校验后立即删除答案，避免同一 id+answer 在 TTL 内被重放。
	return c.Base64Captcha.Verify(id, answer, true)
}
