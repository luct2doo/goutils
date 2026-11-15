package jwt

import (
	"errors"
	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/logger"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	jwtpkg "github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenExpired           error = errors.New("令牌已过期")
	ErrTokenExpiredMaxRefresh error = errors.New("令牌已过最大刷新时间")
	ErrTokenMalformed         error = errors.New("请求令牌格式有误")
	ErrTokenInvalid           error = errors.New("请求令牌无效")
	ErrHeaderEmpty            error = errors.New("需要认证才能访问！")
	ErrHeaderMalformed        error = errors.New("请求头中 Authorization 格式有误")
)

type JWT struct {
	SignKey    []byte
	MaxRefresh time.Duration
	app        *app.App
	appCfg     *config.App
	jwtCfg     *config.JWT
}

type UserInfo struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type JWTCustomClaims struct {
	UserInfo
	ExpireAtTime int64 `json:"expire_time"`
	jwtpkg.RegisteredClaims
}

func NewJWT(jwtCfg *config.JWT, appCfg *config.App, app *app.App) *JWT {
	return &JWT{
		SignKey:    []byte(jwtCfg.Secret),
		MaxRefresh: time.Duration(jwtCfg.MaxRefreshTime) * time.Minute,
		app:        app,
		jwtCfg:     jwtCfg,
		appCfg:     appCfg,
	}
}

// ParserToken 解析 Token，中间件中调用
func (jwt *JWT) ParserToken(c *gin.Context) (*JWTCustomClaims, error) {
	tokenString, parseErr := jwt.getTokenFromHeader(c)
	if parseErr != nil {
		return nil, parseErr
	}

	token, err := jwt.parseTokenString(tokenString)
	if err != nil {
		// v5 推荐使用 errors.Is 来判断错误类型
		if errors.Is(err, jwtpkg.ErrTokenExpired) {
			// 如果是过期错误，我们仍然尝试提取 claims，供无感刷新判断使用
			if token != nil && token.Claims != nil {
				if claims, ok := token.Claims.(*JWTCustomClaims); ok {
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

	return claims, nil
}

func (jwt *JWT) RefreshToken(c *gin.Context) (string, error) {
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

	// 从 token 中解析出 claims 数据
	claims, ok := token.Claims.(*JWTCustomClaims)
	if !ok {
		return "", ErrTokenInvalid
	}

	// 检查是否过了最大允许刷新时间
	expireTime := time.Unix(claims.ExpireAtTime, 0)
	if jwt.app.TimeNowInTimezone().Sub(expireTime) > jwt.MaxRefresh {
		return "", ErrTokenExpiredMaxRefresh
	}

	// 开始签发新的 Token
	return jwt.IssueToken(UserInfo{
		UserID:   claims.UserID,
		Username: claims.Username,
	}), nil
}

func (jwt *JWT) AddNeedRefreshHeader(c *gin.Context, claims *JWTCustomClaims) {
	if claims.ExpireAtTime-jwt.app.TimeNowInTimezone().Unix() < jwt.jwtCfg.RefreshThreshold {
		c.Header("X-Token-Refresh", "true")
	}
}

// IssueToken 生成 Token，在登录成功时调用
func (jwt *JWT) IssueToken(info UserInfo) string {
	expireAtTime := jwt.expireAtTime()
	claims := JWTCustomClaims{
		UserInfo{
			UserID:   info.UserID,
			Username: info.Username,
		},
		expireAtTime,
		jwtpkg.RegisteredClaims{
			Issuer:    jwt.appCfg.Name,
			NotBefore: jwtpkg.NewNumericDate(jwt.app.TimeNowInTimezone()),
			IssuedAt:  jwtpkg.NewNumericDate(jwt.app.TimeNowInTimezone()),
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

// createToken 创建 Token，内部使用，外部请调用 IssueToken
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

// parseTokenString 使用 jwtpkg.ParseWithClaims 解析 Token
func (jwt *JWT) parseTokenString(tokenString string) (*jwtpkg.Token, error) {
	// 接收一个 token 字符串
	// 使用预设的密钥验证这个 token 的签名
	// 将 token 中的数据解析到 JWTCustomClaims 结构体中
	// 返回解析结果
	return jwtpkg.ParseWithClaims(tokenString, &JWTCustomClaims{}, func(token *jwtpkg.Token) (any, error) {
		return jwt.SignKey, nil
	})
}

// getTokenFromHeader 使用 jwtpkg.ParseWithClaims 解析 Token
// Authorization:Bearer xxxxx
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
