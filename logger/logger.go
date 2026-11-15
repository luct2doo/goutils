package logger

import (
	"encoding/json"
	"fmt"
	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *Logger

type Logger struct {
	*zap.Logger
	// wrapped 专供包级日志函数使用。包级函数比实例方法多一层包装，
	// 需要额外跳一帧才能把 caller 指到真实调用点；在初始化时缓存，避免每次调用 clone。
	wrapped *zap.Logger
	app     *app.App
	logCfg  *config.Log
}

func NewLogger(logCfg *config.Log, app *app.App) *Logger {
	writeSyncer := getLogWriter(
		logCfg.Filename,
		logCfg.MaxSize,
		logCfg.MaxBackups,
		logCfg.MaxAge,
		logCfg.Compress,
		logCfg.Type,
		app,
	)
	// 设置日志等级
	logLevel := new(zapcore.Level)
	if err := logLevel.UnmarshalText([]byte(logCfg.Level)); err != nil {
		fmt.Println("日志初始化错误，日志级别设置有误。请修改 config/log.go 文件中的 log.level 配置项")
	}
	// 初始化 core
	core := zapcore.NewCore(getEncoder(app), writeSyncer, logLevel)
	// 初始化 Logger
	//
	// 基础 logger 的 callerSkip 保持 0：zap 默认就把 caller 记为直接调用 Info/Debug 的位置，
	// 这样实例方法 l.Info(...) 天然准确。包级函数多一层封装，改用 wrapped（skip=1）。
	// 一个固定 skip 无法同时满足两种调用形态，这也是过去 caller 指到 runtime/proc.go 的原因。
	zapLogger := zap.New(core,
		zap.AddCaller(),                   // 调用文件和行号，内部使用 runtime.Caller
		zap.AddStacktrace(zap.ErrorLevel), // Error 时才会显示 stacktrace
	)
	// 将自定义的 logger 替换为全局的 logger
	zap.ReplaceGlobals(zapLogger)

	// 创建Logger实例
	loggerInstance := &Logger{
		Logger:  zapLogger,
		wrapped: zapLogger.WithOptions(zap.AddCallerSkip(1)),
		app:     app,
		logCfg:  logCfg,
	}

	// 重要：将实例赋值给全局变量
	logger = loggerInstance

	return loggerInstance
}

func getEncoder(app *app.App) zapcore.Encoder {
	// 日志格式规则
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller", // 代码调用，如 paginator/paginator.go:148
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,      // 每行日志的结尾添加 "\n"
		EncodeLevel:    zapcore.CapitalLevelEncoder,    // 日志级别名称大写，如 ERROR、INFO
		EncodeTime:     customTimeEncoder,              // 时间格式，我们自定义为 2006-01-02 15:04:05
		EncodeDuration: zapcore.SecondsDurationEncoder, // 执行时间，以秒为单位
		EncodeCaller:   zapcore.ShortCallerEncoder,     // Caller 短格式，如：types/converter.go:17，长格式为绝对路径
	}

	// 本地环境配置
	if app.IsLocal() {
		// 终端输出的关键词高亮
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		// 本地设置内置的 Console 解码器（支持 stacktrace 换行）
		return zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 线上环境使用 JSON 编码器
	return zapcore.NewJSONEncoder(encoderConfig)
}

func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format(time.DateTime))
}

func getLogWriter(filename string, maxSize, maxBackup, maxAge int, compress bool, logType string, app *app.App) zapcore.WriteSyncer {
	// 如果配置了按照日期记录日志文件
	if logType == "daily" {
		logname := time.Now().Format(time.DateOnly + ".log")
		filename = strings.ReplaceAll(filename, "logs.log", logname)
	}

	// 滚动日志，详见 config/log.go
	lumberjackLogger := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackup,
		MaxAge:     maxAge,
		Compress:   compress,
	}

	if app.IsLocal() {
		// 本地开发终端打印和记录文件
		return zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), zapcore.AddSync(lumberjackLogger))
	} else {
		// 生产环境只记录文件
		return zapcore.AddSync(lumberjackLogger)
	}
}

func Dump(value any, msg ...string) {
	valueString := jsonString(value)
	// 判断第二个参数是否传参 msg
	if len(msg) > 0 {
		namedLogger().Warn("Dump", zap.String(msg[0], valueString))
	} else {
		namedLogger().Warn("Dump", zap.String("data", valueString))
	}
}

// LogIf 当 err != nil 时记录 error 等级的日志。
// 未调用 NewLogger 前（单元测试、早于日志初始化的启动期）退回 zap 全局 logger（默认 Nop），不 panic。
func LogIf(err error) {
	if err == nil {
		return
	}
	namedLogger().Error("Error Occurred:", zap.Error(err))
}

// LogWarnIf 当 err != nil 时记录 warning 等级的日志
func LogWarnIf(err error) {
	if err == nil {
		return
	}
	namedLogger().Warn("Error Occurred:", zap.Error(err))
}

// LogInfoIf 当 err != nil 时记录 info 等级的日志
func LogInfoIf(err error) {
	if err == nil {
		return
	}
	namedLogger().Info("Error Occurred:", zap.Error(err))
}

// base 返回实际可用的 logger：未调用 NewLogger 前（单测、启动早期）退回 zap 全局 logger。
// 所有导出的日志函数都必须经过它，否则 logger 为 nil 时会直接 panic。
func base() *zap.Logger {
	if logger != nil && logger.Logger != nil {
		return logger.Logger
	}
	return zap.L()
}

// namedLogger 供所有包级日志函数使用：它们比实例方法多一层包装，
// 因此额外跳过 1 帧，让 caller 指到「调用 logger.Info(...) 的业务代码」而不是本文件。
// 直接调用实例方法 l.Info(...) 时请用 base()（即 logger.Logger），不要经过这里。
func namedLogger() *zap.Logger {
	if logger != nil && logger.wrapped != nil {
		return logger.wrapped
	}
	// NewLogger 之前的兜底路径，同样补上那一帧
	return zap.L().WithOptions(zap.AddCallerSkip(1))
}

// Debug 调试日志，详尽的程序日志
// 调用示例：
// logger.Debug("Database", zap.String("sql", sql))
func Debug(moduleName string, fields ...zap.Field) {
	namedLogger().Debug(moduleName, fields...)
}

// Info 告知类日志
func Info(moduleName string, fields ...zap.Field) {
	namedLogger().Info(moduleName, fields...)
}

// Warn 警告类
func Warn(moduleName string, fields ...zap.Field) {
	namedLogger().Warn(moduleName, fields...)
}

// Error 错误时记录，不应该中断程序，查看日志时重点关注
func Error(moduleName string, fields ...zap.Field) {
	namedLogger().Error(moduleName, fields...)
}

// Fatal 级别同 Error(), 写完 log 后调用 os.Exit(1) 退出程序
func Fatal(moduleName string, fields ...zap.Field) {
	namedLogger().Fatal(moduleName, fields...)
}

// DebugString 记录一条字符串类型的 debug 日志，调用示例：
// logger.DebugString("SMS", "短信内容", string(result.RawResponse))
func DebugString(moduleName, name, msg string) {
	namedLogger().Debug(moduleName, zap.String(name, msg))
}

func InfoString(moduleName, name, msg string) {
	namedLogger().Info(moduleName, zap.String(name, msg))
}

func WarnString(moduleName, name, msg string) {
	namedLogger().Warn(moduleName, zap.String(name, msg))
}

func ErrorString(moduleName, name, msg string) {
	namedLogger().Error(moduleName, zap.String(name, msg))
}

func FatalString(moduleName, name, msg string) {
	namedLogger().Fatal(moduleName, zap.String(name, msg))
}

// DebugJSON 记录对象类型的 debug 日志，使用 json.Marshal 进行编码。调用示例：
// logger.DebugJSON("Auth", "读取登录用户", auth.CurrentUser())
func DebugJSON(moduleName, name string, value any) {
	namedLogger().Debug(moduleName, zap.String(name, jsonString(value)))
}

func InfoJSON(moduleName, name string, value any) {
	namedLogger().Info(moduleName, zap.String(name, jsonString(value)))
}

func WarnJSON(moduleName, name string, value any) {
	namedLogger().Warn(moduleName, zap.String(name, jsonString(value)))
}

func ErrorJSON(moduleName, name string, value any) {
	namedLogger().Error(moduleName, zap.String(name, jsonString(value)))
}

func FatalJSON(moduleName, name string, value any) {
	namedLogger().Fatal(moduleName, zap.String(name, jsonString(value)))
}

func jsonString(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		namedLogger().Error("Logger", zap.String("JSON marshal error", err.Error()))
	}
	return string(b)
}
