package jwt

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
)

func TestAddNeedRefreshHeaderUsesSeconds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	appCfg := &config.App{Name: "jwt-test", Timezone: "Asia/Shanghai"}
	jwtCfg := &config.JWT{Secret: "test-secret-at-least-32-bytes-long!!", ExpireTime: 15, RefreshThreshold: 60}
	j := NewJWT(jwtCfg, appCfg, app.NewApp(appCfg))

	// 距过期还有 30 秒 < 阈值 60 秒 → 需要提示刷新
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	claims := &JWTCustomClaims{ExpireAtTime: j.app.TimeNowInTimezone().Add(30 * time.Second).Unix()}
	j.AddNeedRefreshHeader(c, claims)
	if got := w.Header().Get("X-Token-Refresh"); got != "true" {
		t.Errorf("距过期 30s（阈值 60s）应设置刷新头，实际 %q", got)
	}

	// 距过期还有 120 秒 > 阈值 → 不提示
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("GET", "/", nil)
	claims2 := &JWTCustomClaims{ExpireAtTime: j.app.TimeNowInTimezone().Add(120 * time.Second).Unix()}
	j.AddNeedRefreshHeader(c2, claims2)
	if got := w2.Header().Get("X-Token-Refresh"); got != "" {
		t.Errorf("距过期 120s（阈值 60s）不应设置刷新头，实际 %q", got)
	}
}
