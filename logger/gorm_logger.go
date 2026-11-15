package logger

import (
	"context"
	"errors"
	"github.com/luct2doo/goutils/helpers"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// GormLogger 操作对象，实现 gormlogger.Interface
type GormLogger struct {
	ZapLogger     *zap.Logger
	SlowThreshold time.Duration
}

// NewGormLogger 外部调用。实例化一个 GormLogger 对象，示例：
//
//	DB, err := gorm.Open(dbConfig, &gorm.Config{
//	  Logger: logger.NewGormLogger(),
//	})
func NewGormLogger(logger *Logger) *GormLogger {
	return &GormLogger{
		ZapLogger:     logger.Logger,          // 使用全局的 logger.Logger 对象
		SlowThreshold: 200 * time.Millisecond, // 慢查询阈值，单位为千分之一秒
	}
}

// LogMode 实现 gormlogger.Interface 的 LogMode 方法
func (l GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return GormLogger{
		ZapLogger:     l.ZapLogger,
		SlowThreshold: l.SlowThreshold,
	}
}

// Info 实现 gormlogger.Interface 的 Info 方法
func (l GormLogger) Info(ctx context.Context, str string, args ...any) {
	l.logger().Sugar().Debugf(str, args...)
}

// Warn 实现 gormlogger.Interface 的 Warn 方法
func (l GormLogger) Warn(ctx context.Context, str string, args ...any) {
	l.logger().Sugar().Warnf(str, args...)
}

// Error 实现 gormlogger.Interface 的 Error 方法
func (l GormLogger) Error(ctx context.Context, str string, args ...any) {
	l.logger().Sugar().Errorf(str, args...)
}

// Trace 实现 gormlogger.Interface 的 Trace 方法
func (l GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {

	// 获取运行时间
	elapsed := time.Since(begin)
	// 获取 SQL 请求和返回条数
	sql, rows := fc()

	// 通用字段
	logFields := []zap.Field{
		zap.String("sql", sql),
		zap.String("time", helpers.MicrosecondsStr(elapsed)),
		zap.Int64("rows", rows),
	}

	// Gorm 错误
	if err != nil {
		// 记录未找到的错误使用 warning 等级
		if errors.Is(err, gorm.ErrRecordNotFound) {
			l.logger().Warn("Database ErrRecordNotFound", logFields...)
		} else {
			// 其他错误使用 error 等级
			logFields = append(logFields, zap.Error(err))
			l.logger().Error("Database Error", logFields...)
		}
	}

	// 慢查询日志
	if l.SlowThreshold != 0 && elapsed > l.SlowThreshold {
		l.logger().Warn("Database Slow Log", logFields...)
	}

	// 记录所有 SQL 请求
	l.logger().Debug("Database Query", logFields...)
}

// logger 返回一个 caller 指向「触发本次 GORM 回调的业务代码」的 zap logger。
//
// 换算关系：本方法里 runtime.Caller(i) 的 0 号帧是本方法、1 号帧是 Trace/Info/... ，
// 2 号帧起才是调用方（GORM 内部帧或业务代码）。zap 在 skip=0 时把 caller 记为
// 1 号帧（即 l.logger().Debug(...) 所在行），因此要让 caller 落到第 i 帧，
// 需要 AddCallerSkip(i-1)。
//
// 旧实现用 AddCallerSkip(-2) 补偿基础 logger 的 AddCallerSkip(2)，
// 两处魔法数强耦合；基础 logger 一改（或 Go 版本改变内联/栈形态）就整体偏移，
// caller 会指到 testing.go / asm_arm64.s。现在只依赖本方法自己的栈，不再耦合。
func (l GormLogger) logger() *zap.Logger {
	var (
		gormPackage    = filepath.Join("gorm.io", "gorm")
		zapgormPackage = filepath.Join("moul.io", "zapgorm2")
	)

	for i := 2; i < 15; i++ {
		_, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}
		// 跳过 GORM 内部帧；测试文件属于业务调用方，不能跳过，否则 caller 会落到 testing 框架
		if strings.Contains(file, gormPackage) || strings.Contains(file, zapgormPackage) {
			continue
		}
		return l.ZapLogger.WithOptions(zap.AddCallerSkip(i - 1))
	}
	return l.ZapLogger
}
