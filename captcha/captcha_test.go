package captcha

import (
	"testing"

	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"

	base64Captcha "github.com/mojocn/base64Captcha"
)

// stubStore 实现 base64Captcha.Store，避免测试依赖 Redis。
type stubStore struct {
	answer    string
	lastClear bool
}

func (s *stubStore) Set(_, value string) error { s.answer = value; return nil }
func (s *stubStore) Get(_ string, clear bool) string {
	// base64Captcha.Captcha.Verify 实际走的是 Store.Get(id, clear)
	s.lastClear = clear
	return s.answer
}
func (s *stubStore) Verify(_ string, answer string, clear bool) bool {
	s.lastClear = clear
	return s.answer == answer
}

func newTestService(env string, cfg *config.Captcha, store *stubStore) *CaptchaService {
	appCfg := &config.App{Name: "captcha-test", Env: env, Timezone: "Asia/Shanghai"}
	driver := base64Captcha.NewDriverDigit(40, 100, 4, 0.7, 80)
	return &CaptchaService{
		Base64Captcha: base64Captcha.NewCaptcha(driver, store),
		captchaCfg:    cfg,
		appCfg:        appCfg,
		app:           app.NewApp(appCfg),
	}
}

func TestVerifyCaptchaBypassRequiresAllConditions(t *testing.T) {
	store := &stubStore{answer: "1234"}

	// 1. TestingEnabled=true 但 TestingKey 为空：空 id 不能直接通过
	svc := newTestService("local", &config.Captcha{TestingEnabled: true, TestingKey: ""}, store)
	if svc.VerifyCaptcha("", "whatever") {
		t.Error("TestingKey 为空时，空 id 不应通过")
	}

	// 2. TestingEnabled=false：即使 id 命中 TestingKey 也不放行
	svc = newTestService("local", &config.Captcha{TestingEnabled: false, TestingKey: "skip"}, store)
	if svc.VerifyCaptcha("skip", "whatever") {
		t.Error("TestingEnabled=false 时不应放行")
	}

	// 3. 四个条件都满足：放行
	svc = newTestService("local", &config.Captcha{TestingEnabled: true, TestingKey: "skip"}, store)
	if !svc.VerifyCaptcha("skip", "whatever") {
		t.Error("条件满足时应放行")
	}

	// 4. 生产环境：不放行
	svc = newTestService("production", &config.Captcha{TestingEnabled: true, TestingKey: "skip"}, store)
	if svc.VerifyCaptcha("skip", "whatever") {
		t.Error("生产环境不应放行")
	}
}

func TestVerifyCaptchaClearsAnswer(t *testing.T) {
	store := &stubStore{answer: "1234"}
	svc := newTestService("local", &config.Captcha{TestingEnabled: false}, store)

	if !svc.VerifyCaptcha("id", "1234") {
		t.Fatal("正确答案应校验通过")
	}
	if !store.lastClear {
		t.Error("校验后应删除答案（clear=true），避免重放")
	}
}
