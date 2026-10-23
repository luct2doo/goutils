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
func (c *CaptchaService) VerifyCaptcha(id, answer string) (match bool) {
	// TODO(REMOVE-BEFORE-RELEASE): 【调试后门，项目上线前必须删除】
	//
	// 作用：只要 captcha_id 等于配置项 captcha.testing_key（当前配置值是 "captcha_skip_test"），
	// 无论 answer 填什么都直接判定通过。用于本地 / 联调用 Bruno、curl 调登录接口时，
	// 不想每次都肉眼识别图形验证码。
	//
	// 风险（为什么要删）：
	//   - 放行条件是 Env != "production"（IsProduction 是精确字符串比较），
	//     env 写成 prod / Production / prd 或留空，都会让这个后门在生产环境生效；
	//   - 预发 / 测试环境只要能被公网访问，验证码就等于不存在，撞库只剩登录失败节流一层防御。
	//
	// 清理清单（全局搜索关键字 REMOVE-BEFORE-RELEASE）：
	//   1. 删除下面这个 if 分支；
	//   2. 删除 config.Captcha 的 TestingKey 字段（goutils/config/captcha.go）；
	//   3. 删除 server/config/config.yml 与 config.example.yml 里的 captcha.testing_key；
	//   4. 清理前端 / 接口调试脚本里硬编码的 captcha_id: "captcha_skip_test"。
	if !c.app.IsProduction() && id == c.captchaCfg.TestingKey {
		return true
	}
	return c.Base64Captcha.Verify(id, answer, false)
}
