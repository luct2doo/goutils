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

func (c *CaptchaService) VerifyCaptcha(id, answer string) (match bool) {
	if !c.app.IsProduction() && id == c.captchaCfg.TestingKey {
		return true
	}
	return c.Base64Captcha.Verify(id, answer, false)
}
