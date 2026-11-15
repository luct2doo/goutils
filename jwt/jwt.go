package jwt

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	jwtpkg "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/logger"
)

var (
	ErrTokenExpired           error = errors.New("令牌已过期")
	ErrTokenExpiredMaxRefresh error = errors.New("令牌已过最大刷新时间")
	ErrTokenMalformed         error = errors.New("请求令牌格式有误")
	ErrTokenInvalid           error = errors.New("请求令牌无效")
	ErrHeaderEmpty            error = errors.New("需要认证才能访问！")
	ErrHeaderMalformed        error = errors.New("请求头中 Authorization 格式有误")

	// ErrSessionExpired 从登录时刻算起已超过 MaxSessionAge，必须重新登录。
	// 用于阻止被盗 token 通过反复刷新无限续期。
	ErrSessionExpired error = errors.New("会话已到期，请重新登录")
	// ErrTokenRevoked 供 RevocationChecker 实现返回；库本身只透传错误，不强制使用该哨兵。
	ErrTokenRevoked error = errors.New("令牌已被吊销")
)

// RevocationChecker 由业务侧注入，判断某个 jti 是否已被吊销（登出 / 改密 / 封禁 / 刷新后的旧 jti）。
// 每次请求都会调用，实现应走 Redis 之类的低开销存储；返回非 nil 即拒绝该 token。
//
// 业务侧示例（key 的 TTL 至少要覆盖 token 的最大存活时间）：
//
//	func(ctx context.Context, c *jwt.JWTCustomClaims) error {
//	    if c.ID == "" { return nil } // 兼容没有 jti 的历史 token
//	    n, err := rdb.Exists(ctx, "jwt:blacklist:"+c.ID).Result()
//	    if err != nil { return err } // Redis 故障时按 fail-closed 处理
//	    if n > 0 { return jwt.ErrTokenRevoked }
//	    return nil
//	}
//
// 另一种做法是不用黑名单，而是在 user 表存 token_version，claims 里带版本号，
// 校验时与库里的值比对；改密 / 强制下线只需 +1，一次改动作废该用户全部 token。
type RevocationChecker func(ctx context.Context, claims *JWTCustomClaims) error

// RefreshHook 在续签前调用，业务侧可把旧 jti 拉黑，实现 refresh token 一次性轮换
// （旧 jti 被二次使用即视为盗用，可顺带吊销整条会话链）。
// old.ID 是旧 jti，newJTI 是即将签发的新 jti；返回 error 则不签发新 token。
type RefreshHook func(ctx context.Context, old *JWTCustomClaims, newJTI string) error

// Option 构造期注入的可选能力。所有注入都在 NewJWTWithOptions 内完成，
// 避免运行时修改 *JWT 造成中间件并发读的数据竞争。
type Option func(*JWT)

// WithRevocationChecker 注入吊销校验，业务侧决定黑名单存哪、怎么判。
func WithRevocationChecker(fn RevocationChecker) Option {
	return func(j *JWT) { j.revocationChecker = fn }
}

// WithRefreshHook 注入续签前钩子，用于旧 jti 一次性轮换。
func WithRefreshHook(fn RefreshHook) Option {
	return func(j *JWT) { j.refreshHook = fn }
}

type JWT struct {
	SignKey    []byte
	MaxRefresh time.Duration
	// MaxSessionAge 从登录时刻算起的绝对会话上限，0 表示不限制。
	MaxSessionAge time.Duration
	app           *app.App
	appCfg        *config.App
	jwtCfg        *config.JWT

	revocationChecker RevocationChecker
	refreshHook       RefreshHook
}

type UserInfo struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type JWTCustomClaims struct {
	UserInfo
	ExpireAtTime int64 `json:"expire_time"`
	// AuthTime 登录时刻（Unix 秒）。刷新时透传、绝不重置，用于计算绝对会话上限。
	// 历史 token 没有该字段，校验时回退到 RegisteredClaims.IssuedAt。
	AuthTime int64 `json:"auth_time"`
	// RegisteredClaims.ID 即标准 jti，每次签发都会换新值，供吊销 / 一次性轮换使用。
	jwtpkg.RegisteredClaims
}

// NewJWT 保持原有签名。需要吊销或一次性轮换能力时改用 NewJWTWithOptions。
func NewJWT(jwtCfg *config.JWT, appCfg *config.App, a *app.App) *JWT {
	return NewJWTWithOptions(jwtCfg, appCfg, a)
}

// NewJWTWithOptions 在构造期注入可选能力。MaxSessionAge 为 0 时行为与旧版一致。
func NewJWTWithOptions(jwtCfg *config.JWT, appCfg *config.App, a *app.App, opts ...Option) *JWT {
	j := &JWT{
		SignKey:       []byte(jwtCfg.Secret),
		MaxRefresh:    time.Duration(jwtCfg.MaxRefreshTime) * time.Minute,
		MaxSessionAge: time.Duration(jwtCfg.MaxSessionAge) * time.Minute,
		app:           a,
		appCfg:        appCfg,
		jwtCfg:        jwtCfg,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(j)
		}
	}
	return j
}

// ParserToken 解析 Token，中间件中调用。
// 除签名 / 有效期外，还会过绝对会话上限与吊销校验，因此登出、改密、封禁可以即时生效。
func (jwt *JWT) ParserToken(c *gin.Context) (*JWTCustomClaims, error) {
	tokenString, parseErr := jwt.getTokenFromHeader(c)
	if parseErr != nil {
		return nil, parseErr
	}

	ctx := c.Request.Context()
	token, err := jwt.parseTokenString(tokenString)
	if err != nil {
		// v5 推荐使用 errors.Is 来判断错误类型
		if errors.Is(err, jwtpkg.ErrTokenExpired) {
			// 如果是过期错误，我们仍然尝试提取 claims，供无感刷新判断使用；
			// 但过期 token 同样要过会话上限与吊销检查，不能成为绕过校验的通道。
			if token != nil && token.Claims != nil {
				if claims, ok := token.Claims.(*JWTCustomClaims); ok {
					if vErr := jwt.validateClaims(ctx, claims); vErr != nil {
						return claims, vErr
					}
					return claims, ErrTokenExpired
				}
			}
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwtpkg.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		}
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*JWTCustomClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	if err := jwt.validateClaims(ctx, claims); err != nil {
		return claims, err
	}
	return claims, nil
}

func (jwt *JWT) RefreshToken(c *gin.Context) (string, error) {
	ctx := c.Request.Context()
	tokenString, parseErr := jwt.getTokenFromHeader(c)
	if parseErr != nil {
		return "", parseErr
	}

	token, err := jwt.parseTokenString(tokenString)
	if err != nil {
		// token 过期可以续签，其他错误则不可以
		if !errors.Is(err, jwtpkg.ErrTokenExpired) {
			return "", err
		}
	}
	if token == nil {
		return "", ErrTokenInvalid
	}

	// 从 token 中解析出 claims 数据
	claims, ok := token.Claims.(*JWTCustomClaims)
	if !ok {
		return "", ErrTokenInvalid
	}

	// 检查是否过了最大允许刷新时间（保留原有的「过期后续签宽限」语义）
	expireTime := time.Unix(claims.ExpireAtTime, 0)
	if jwt.app.TimeNowInTimezone().Sub(expireTime) > jwt.MaxRefresh {
		return "", ErrTokenExpiredMaxRefresh
	}

	// 绝对会话上限 + 吊销检查：新 token 会重置 exp，若不设绝对上限就能无限续期。
	if err := jwt.validateClaims(ctx, claims); err != nil {
		return "", err
	}

	authAt := jwt.sessionStart(claims)
	newJTI := uuid.NewString()
	if jwt.refreshHook != nil {
		if err := jwt.refreshHook(ctx, claims, newJTI); err != nil {
			return "", err
		}
	}

	// 透传会话起点、换新 jti，绝不重置 auth_time
	return jwt.IssueTokenWithSession(UserInfo{
		UserID:   claims.UserID,
		Username: claims.Username,
	}, authAt, newJTI), nil
}

// validateClaims 每次请求与每次续签都要过的统一校验。
func (jwt *JWT) validateClaims(ctx context.Context, claims *JWTCustomClaims) error {
	if claims == nil {
		return ErrTokenInvalid
	}
	if jwt.MaxSessionAge > 0 {
		authAt := jwt.sessionStart(claims)
		// 无法判定会话起点时按已过期处理（fail-closed），避免留下无上限续期的口子
		if authAt.IsZero() || jwt.app.TimeNowInTimezone().Sub(authAt) > jwt.MaxSessionAge {
			return ErrSessionExpired
		}
	}
	if jwt.revocationChecker != nil {
		return jwt.revocationChecker(ctx, claims)
	}
	return nil
}

// sessionStart 优先用 auth_time；历史 token 回退到标准 iat。
func (jwt *JWT) sessionStart(claims *JWTCustomClaims) time.Time {
	if claims.AuthTime > 0 {
		return time.Unix(claims.AuthTime, 0)
	}
	if claims.IssuedAt != nil {
		return claims.IssuedAt.Time
	}
	return time.Time{}
}

func (jwt *JWT) AddNeedRefreshHeader(c *gin.Context, claims *JWTCustomClaims) {
	if claims.ExpireAtTime-jwt.app.TimeNowInTimezone().Unix() < jwt.jwtCfg.RefreshThreshold {
		c.Header("X-Token-Refresh", "true")
	}
}

// IssueToken 生成 Token，在登录成功时调用：开启一个全新会话（auth_time = now，新 jti）。
func (jwt *JWT) IssueToken(info UserInfo) string {
	return jwt.IssueTokenWithSession(info, jwt.app.TimeNowInTimezone(), uuid.NewString())
}

// IssueTokenWithSession 按指定会话起点与 jti 签发，供刷新或跨系统迁移使用。
// authTime 为零值、jti 为空时自动补全，调用方可以只关心其一。
func (jwt *JWT) IssueTokenWithSession(info UserInfo, authTime time.Time, jti string) string {
	now := jwt.app.TimeNowInTimezone()
	if authTime.IsZero() {
		authTime = now
	}
	if jti == "" {
		jti = uuid.NewString()
	}

	expireAtTime := jwt.expireAtTime()
	claims := JWTCustomClaims{
		UserInfo: UserInfo{
			UserID:   info.UserID,
			Username: info.Username,
		},
		ExpireAtTime: expireAtTime,
		AuthTime:     authTime.Unix(),
		RegisteredClaims: jwtpkg.RegisteredClaims{
			ID:        jti, // 标准 jti，吊销与一次性轮换都靠它
			Issuer:    jwt.appCfg.Name,
			NotBefore: jwtpkg.NewNumericDate(now),
			IssuedAt:  jwtpkg.NewNumericDate(now),
			ExpiresAt: jwtpkg.NewNumericDate(time.Unix(expireAtTime, 0)),
		},
	}

	token, err := jwt.createToken(claims)
	if err != nil {
		logger.LogIf(err)
		return ""
	}

	return token
}

// createToken 创建 Token，内部使用，外部请调用 IssueToken / IssueTokenWithSession
func (jwt *JWT) createToken(claims JWTCustomClaims) (string, error) {
	// 使用HS256算法进行token生成
	token := jwtpkg.NewWithClaims(jwtpkg.SigningMethodHS256, claims)
	return token.SignedString(jwt.SignKey)
}

func (jwt *JWT) expireAtTime() int64 {
	timeNow := jwt.app.TimeNowInTimezone()

	var expireTime int64
	if jwt.appCfg.Debug {
		expireTime = jwt.jwtCfg.DebugExpireTime
	} else {
		expireTime = jwt.jwtCfg.ExpireTime
	}

	expire := time.Duration(expireTime) * time.Minute
	return timeNow.Add(expire).Unix()
}

// parseTokenString 使用 jwtpkg.ParseWithClaims 解析 Token，并锁定签名算法为 HS256。
func (jwt *JWT) parseTokenString(tokenString string) (*jwtpkg.Token, error) {
	// 接收一个 token 字符串
	// 使用预设的密钥验证这个 token 的签名
	// 将 token 中的数据解析到 JWTCustomClaims 结构体中
	// 返回解析结果
	return jwtpkg.ParseWithClaims(tokenString, &JWTCustomClaims{}, func(token *jwtpkg.Token) (any, error) {
		return jwt.SignKey, nil
	}, jwtpkg.WithValidMethods([]string{jwtpkg.SigningMethodHS256.Alg()}))
}

// getTokenFromHeader 从 Authorization: Bearer xxxxx 中提取 token
func (jwt *JWT) getTokenFromHeader(c *gin.Context) (string, error) {
	authHeader := c.Request.Header.Get("Authorization")
	if authHeader == "" {
		return "", ErrHeaderEmpty
	}
	// split by blank
	parts := strings.SplitN(authHeader, " ", 2)
	if !(len(parts) == 2 && parts[0] == "Bearer") {
		return "", ErrHeaderMalformed
	}
	return parts[1], nil
}
