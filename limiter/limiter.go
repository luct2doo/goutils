package limiter

import (
	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/logger"
	"github.com/luct2doo/goutils/redis"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	limiterlib "github.com/ulule/limiter/v3"
	sredis "github.com/ulule/limiter/v3/drivers/store/redis"
)

type Limiter struct {
	redisClient *redis.RedisClient
	store       limiterlib.Store
	limiters    sync.Map // 缓存不同 rate 的限流器实例
}

func NewLimiter(redisClient *redis.RedisClient, appCfg *config.App) *Limiter {
	// 初始化时创建一次 store 即可
	store, err := sredis.NewStoreWithOptions(redisClient.Client, limiterlib.StoreOptions{
		Prefix: appCfg.Name + ":limiter",
	})
	if err != nil {
		logger.LogIf(err)
	}

	// 因为采用的是 key: value 这种按字段名赋值方式，这种方式允许只初始化部分字段，可以省略 limiters 字段的赋值。没有写出来的字段，Go 编译器会自动用它们的“零值”来填充。
	// sync.Map 和 map 的不同:
	// 普通 map 的陷阱： 如果你定义的是普通的 limiters map[string]any，它的零值是 nil。如果你在初始化时省略了它，后续代码调用 limiters["key"] = value 时，程序会直接 panic（报错：assignment to entry in nil map），因为普通 map 必须先用 make() 初始化才能写入。
	// sync.Map 的优势： sync.Map 是 Go 为并发场景设计的 Map。sync.Map 的零值就是一个空的、未锁定且完全可用的 Map。 它的内部机制保证了你可以直接对一个零值的 sync.Map 调用 Store()、Load() 等方法，完全不需要像普通 map 那样提前 make()。
	return &Limiter{
		redisClient: redisClient,
		store:       store,
	}
}

// GetKeyIP 获取请求的 IP 地址
func (l *Limiter) GetKeyIP(c *gin.Context) string {
	return c.ClientIP()
}

// GetKeyRouteWithIP
func (l *Limiter) GetKeyRouteWithIP(c *gin.Context) string {
	return l.routeToKeyString(c.FullPath()) + c.ClientIP()
}

// CheckRate 检测请求是否超额
// 参数:
//   - c: gin上下文
//   - key: 限流键
//   - formatted: 限流格式字符串
//   - appCfg: 应用配置
//
// 返回:
//   - limiterlib.Context: 限流上下文
//   - error: 错误信息
func (l *Limiter) CheckRate(c *gin.Context, key, formatted string) (limiterlib.Context, error) {
	var ctx limiterlib.Context

	// 1. 从缓存中获取或创建 Limiter 实例
	limiterObjInterface, ok := l.limiters.Load(formatted)
	if !ok {
		rate, err := limiterlib.NewRateFromFormatted(formatted)
		if err != nil {
			logger.LogIf(err)
			return ctx, err
		}
		limiterObjInterface, _ = l.limiters.LoadOrStore(formatted, limiterlib.New(l.store, rate))
	}

	limiterObj := limiterObjInterface.(limiterlib.Limiter)

	// 2. 执行限流检测
	if c.GetBool("limiter-once") {
		return limiterObj.Peek(c, key)
	} else {
		c.Set("limiter-once", true)
		return limiterObj.Get(c, key)
	}
}

func (l *Limiter) routeToKeyString(routeName string) string {
	routeName = strings.ReplaceAll(routeName, "/", "-")
	routeName = strings.ReplaceAll(routeName, ":", "_")
	return routeName
}
