package jwt

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	jwtpkg "github.com/golang-jwt/jwt/v5"
	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
)

func newTestJWT(t *testing.T, maxSessionAge int64, opts ...Option) *JWT {
	t.Helper()
	gin.SetMode(gin.TestMode)
	appCfg := &config.App{Name: "goutils-test", Timezone: "Asia/Shanghai"}
	jwtCfg := &config.JWT{
		Secret:         "test-secret-at-least-32-bytes-long!!",
		ExpireTime:     15,
		MaxRefreshTime: 60,
		MaxSessionAge:  maxSessionAge,
	}
	return NewJWTWithOptions(jwtCfg, appCfg, app.NewApp(appCfg), opts...)
}

func ctxWithToken(token string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	return c
}

func testUser() UserInfo {
	return UserInfo{UserID: "u-1", Username: "tester"}
}

func TestIssueTokenHasJTIAndAuthTime(t *testing.T) {
	j := newTestJWT(t, 10080)
	raw := j.IssueToken(testUser())
	if raw == "" {
		t.Fatal("签发返回空 token")
	}
	claims, err := j.ParserToken(ctxWithToken(raw))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if claims.ID == "" {
		t.Error("claims.ID (jti) 为空")
	}
	if claims.AuthTime <= 0 {
		t.Error("claims.AuthTime 为空")
	}
}

func TestRefreshKeepsAuthTimeAndRotatesJTI(t *testing.T) {
	j := newTestJWT(t, 10080)
	old := j.IssueToken(testUser())
	oldClaims, err := j.ParserToken(ctxWithToken(old))
	if err != nil {
		t.Fatalf("解析旧 token 失败: %v", err)
	}

	newRaw, err := j.RefreshToken(ctxWithToken(old))
	if err != nil {
		t.Fatalf("续签失败: %v", err)
	}
	newClaims, err := j.ParserToken(ctxWithToken(newRaw))
	if err != nil {
		t.Fatalf("解析新 token 失败: %v", err)
	}
	if newClaims.AuthTime != oldClaims.AuthTime {
		t.Errorf("续签重置了 auth_time: old=%d new=%d", oldClaims.AuthTime, newClaims.AuthTime)
	}
	if newClaims.ID == "" || newClaims.ID == oldClaims.ID {
		t.Errorf("续签未轮换 jti: old=%q new=%q", oldClaims.ID, newClaims.ID)
	}
}

func TestMaxSessionAgeForcesRelogin(t *testing.T) {
	j := newTestJWT(t, 60) // 绝对上限 1 小时
	raw := j.IssueTokenWithSession(testUser(), time.Now().Add(-2*time.Hour), "")
	if raw == "" {
		t.Fatal("签发返回空 token")
	}
	if _, err := j.ParserToken(ctxWithToken(raw)); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("ParserToken 应拒绝过期会话，实际: %v", err)
	}
	if _, err := j.RefreshToken(ctxWithToken(raw)); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("RefreshToken 应拒绝过期会话，实际: %v", err)
	}
}

func TestRevocationCheckerBlocksToken(t *testing.T) {
	var revoked string
	j := newTestJWT(t, 0, WithRevocationChecker(func(_ context.Context, claims *JWTCustomClaims) error {
		if claims.ID != "" && claims.ID == revoked {
			return ErrTokenRevoked
		}
		return nil
	}))

	raw := j.IssueToken(testUser())
	claims, err := j.ParserToken(ctxWithToken(raw))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	revoked = claims.ID

	if _, err := j.ParserToken(ctxWithToken(raw)); !errors.Is(err, ErrTokenRevoked) {
		t.Errorf("ParserToken 未拦截已吊销 jti，实际: %v", err)
	}
	if _, err := j.RefreshToken(ctxWithToken(raw)); !errors.Is(err, ErrTokenRevoked) {
		t.Errorf("RefreshToken 未拦截已吊销 jti，实际: %v", err)
	}
}

func TestRefreshHookRotatesOldJTI(t *testing.T) {
	var oldID, newID string
	j := newTestJWT(t, 0, WithRefreshHook(func(_ context.Context, old *JWTCustomClaims, newJTI string) error {
		oldID, newID = old.ID, newJTI
		return nil
	}))

	raw := j.IssueToken(testUser())
	oldClaims, err := j.ParserToken(ctxWithToken(raw))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	newRaw, err := j.RefreshToken(ctxWithToken(raw))
	if err != nil {
		t.Fatalf("续签失败: %v", err)
	}
	if oldID != oldClaims.ID || newID == "" || newID == oldID {
		t.Fatalf("hook 未拿到正确的 jti: old=%q/%q new=%q", oldClaims.ID, oldID, newID)
	}
	newClaims, err := j.ParserToken(ctxWithToken(newRaw))
	if err != nil {
		t.Fatalf("解析新 token 失败: %v", err)
	}
	if newClaims.ID != newID {
		t.Errorf("新 token 的 jti 与 hook 收到的不一致: %q != %q", newClaims.ID, newID)
	}
}

func TestLegacyTokenFallsBackToIssuedAt(t *testing.T) {
	j := newTestJWT(t, 10080)
	now := j.app.TimeNowInTimezone()
	legacy := JWTCustomClaims{
		UserInfo:     testUser(),
		ExpireAtTime: now.Add(15 * time.Minute).Unix(),
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Issuer:    "goutils-test",
			NotBefore: jwtpkg.NewNumericDate(now),
			IssuedAt:  jwtpkg.NewNumericDate(now.Add(-10 * time.Minute)),
			ExpiresAt: jwtpkg.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
	raw, err := j.createToken(legacy)
	if err != nil {
		t.Fatalf("构造历史 token 失败: %v", err)
	}
	claims, err := j.ParserToken(ctxWithToken(raw))
	if err != nil {
		t.Fatalf("历史 token 应可兼容解析，实际: %v", err)
	}
	if claims.AuthTime != 0 {
		t.Errorf("历史 token 的 auth_time 应为空，实际: %d", claims.AuthTime)
	}
}

func TestRejectsUnexpectedSigningMethod(t *testing.T) {
	j := newTestJWT(t, 0)
	now := j.app.TimeNowInTimezone()
	claims := JWTCustomClaims{
		UserInfo:     testUser(),
		ExpireAtTime: now.Add(15 * time.Minute).Unix(),
		RegisteredClaims: jwtpkg.RegisteredClaims{
			NotBefore: jwtpkg.NewNumericDate(now),
			IssuedAt:  jwtpkg.NewNumericDate(now),
			ExpiresAt: jwtpkg.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
	raw, err := jwtpkg.NewWithClaims(jwtpkg.SigningMethodHS512, claims).SignedString(j.SignKey)
	if err != nil {
		t.Fatalf("构造 HS512 token 失败: %v", err)
	}
	if _, err := j.ParserToken(ctxWithToken(raw)); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("非 HS256 token 应被拒绝，实际: %v", err)
	}
}
