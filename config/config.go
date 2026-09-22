// Package config 定义 goutils 各子包所需的通用配置结构体。
//
// 设计原则：这些结构体只描述「形状」，不关心配置从哪里来（viper / 环境变量 / 硬编码皆可）。
// 使用方（应用侧）负责把配置文件反序列化成这些结构体，再注入到各组件构造函数中。
package config

import "time"

// App 应用基础配置
type App struct {
	Name     string `json:"name" mapstructure:"name" yaml:"name"`
	Version  string `json:"version" mapstructure:"version" yaml:"version"`
	Port     string `json:"port" mapstructure:"port" yaml:"port"`
	Env      string `json:"env" mapstructure:"env" yaml:"env"`
	Timezone string `json:"timezone" mapstructure:"timezone" yaml:"timezone"`
	URL      string `json:"url" mapstructure:"url" yaml:"url"`
	Debug    bool   `json:"debug" mapstructure:"debug" yaml:"debug"`
}

// Log 日志配置
type Log struct {
	Filename   string `json:"filename" mapstructure:"filename" yaml:"filename"`
	MaxSize    int    `json:"max_size" mapstructure:"max_size" yaml:"max_size"`
	MaxBackups int    `json:"max_backups" mapstructure:"max_backups" yaml:"max_backups"`
	MaxAge     int    `json:"max_age" mapstructure:"max_age" yaml:"max_age"`
	Compress   bool   `json:"compress" mapstructure:"compress" yaml:"compress"`
	Type       string `json:"type" mapstructure:"type" yaml:"type"`
	Level      string `json:"level" mapstructure:"level" yaml:"level"`
}

// JWT JWT 签发与校验配置
type JWT struct {
	Secret           string `json:"secret" mapstructure:"secret" yaml:"secret"`
	ExpireTime       int64  `json:"expire_time" mapstructure:"expire_time" yaml:"expire_time"`
	MaxRefreshTime   int64  `json:"max_refresh_time" mapstructure:"max_refresh_time" yaml:"max_refresh_time"`
	DebugExpireTime  int64  `json:"debug_expire_time" mapstructure:"debug_expire_time" yaml:"debug_expire_time"`
	RefreshThreshold int64  `json:"refresh_threshold" mapstructure:"refresh_threshold" yaml:"refresh_threshold"`
	TTL              int64  `json:"ttl" mapstructure:"ttl" yaml:"ttl"`
	DebugTTL         int64  `json:"debug_ttl" mapstructure:"debug_ttl" yaml:"debug_ttl"`
}

// Redis Redis 连接配置（支持 standalone / sentinel）
type Redis struct {
	Mode string `json:"mode" mapstructure:"mode" yaml:"mode"`

	Host     string `json:"host" mapstructure:"host" yaml:"host"`
	Port     int    `json:"port" mapstructure:"port" yaml:"port"`
	Username string `json:"username" mapstructure:"username" yaml:"username"`
	Password string `json:"password" mapstructure:"password" yaml:"password"`
	DB       int    `json:"db" mapstructure:"db" yaml:"db"`
	DBCache  int    `json:"db_cache" mapstructure:"db_cache" yaml:"db_cache"`

	SentinelMasterName string   `json:"sentinel_master_name" mapstructure:"sentinel_master_name" yaml:"sentinel_master_name"`
	SentinelAddrs      []string `json:"sentinel_addrs" mapstructure:"sentinel_addrs" yaml:"sentinel_addrs"`
	SentinelUsername   string   `json:"sentinel_username" mapstructure:"sentinel_username" yaml:"sentinel_username"`
	SentinelPassword   string   `json:"sentinel_password" mapstructure:"sentinel_password" yaml:"sentinel_password"`

	DialTimeout  string `json:"dial_timeout" mapstructure:"dial_timeout" yaml:"dial_timeout"`
	ReadTimeout  string `json:"read_timeout" mapstructure:"read_timeout" yaml:"read_timeout"`
	WriteTimeout string `json:"write_timeout" mapstructure:"write_timeout" yaml:"write_timeout"`

	PoolSize     int `json:"pool_size" mapstructure:"pool_size" yaml:"pool_size"`
	MinIdleConns int `json:"min_idle_conns" mapstructure:"min_idle_conns" yaml:"min_idle_conns"`

	MaxRetries      int    `json:"max_retries" mapstructure:"max_retries" yaml:"max_retries"`
	MinRetryBackoff string `json:"min_retry_backoff" mapstructure:"min_retry_backoff" yaml:"min_retry_backoff"`
	MaxRetryBackoff string `json:"max_retry_backoff" mapstructure:"max_retry_backoff" yaml:"max_retry_backoff"`
}

// MySQL MySQL 连接配置
type MySQL struct {
	Host     string `json:"host" mapstructure:"host" yaml:"host"`
	Port     int    `json:"port" mapstructure:"port" yaml:"port"`
	Username string `json:"username" mapstructure:"username" yaml:"username"`
	Password string `json:"password" mapstructure:"password" yaml:"password"`
	Database string `json:"database" mapstructure:"database" yaml:"database"`
}

// SQLite SQLite 连接配置
type SQLite struct {
	Database string `json:"database" mapstructure:"database" yaml:"database"`
}

// Postgres PostgreSQL 连接配置
type Postgres struct {
	Host     string `json:"host" mapstructure:"host" yaml:"host"`
	Port     int    `json:"port" mapstructure:"port" yaml:"port"`
	Username string `json:"username" mapstructure:"username" yaml:"username"`
	Password string `json:"password" mapstructure:"password" yaml:"password"`
	Database string `json:"database" mapstructure:"database" yaml:"database"`
	SSLMode  string `json:"sslmode" mapstructure:"sslmode" yaml:"sslmode"`
	Timezone string `json:"timezone" mapstructure:"timezone" yaml:"timezone"`
}

// Database 数据库配置聚合
type Database struct {
	Connection      string        `json:"connection" mapstructure:"connection" yaml:"connection"`
	MySQL           MySQL         `json:"mysql" mapstructure:"mysql" yaml:"mysql"`
	SQLite          SQLite        `json:"sqlite" mapstructure:"sqlite" yaml:"sqlite"`
	Postgres        Postgres      `json:"postgres" mapstructure:"postgres" yaml:"postgres"`
	MaxIdleConns    int           `json:"max_idle_conns" mapstructure:"max_idle_conns" yaml:"max_idle_conns"`
	MaxOpenConns    int           `json:"max_open_conns" mapstructure:"max_open_conns" yaml:"max_open_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" mapstructure:"conn_max_lifetime" yaml:"conn_max_lifetime"`
}

// Cache 缓存配置
type Cache struct {
	Driver     string `json:"driver" mapstructure:"driver" yaml:"driver"`
	Prefix     string `json:"prefix" mapstructure:"prefix" yaml:"prefix"`
	ExpireTime int    `json:"expire_time" mapstructure:"expire_time" yaml:"expire_time"`
}

// Captcha 图形验证码配置
type Captcha struct {
	Width           int     `json:"width" mapstructure:"width" yaml:"width"`
	Height          int     `json:"height" mapstructure:"height" yaml:"height"`
	Length          int     `json:"length" mapstructure:"length" yaml:"length"`
	MaxSkew         float64 `json:"max_skew" mapstructure:"max_skew" yaml:"max_skew"`
	DotCount        int     `json:"dot_count" mapstructure:"dot_count" yaml:"dot_count"`
	ExpireTime      int     `json:"expire_time" mapstructure:"expire_time" yaml:"expire_time"`
	DebugExpireTime int     `json:"debug_expire_time" mapstructure:"debug_expire_time" yaml:"debug_expire_time"`

	// TestingEnabled 是否允许「测试用万能验证码」旁路。默认 false。
	//
	// 这是一条完全绕过验证码的通路，必须显式开启才会生效，
	// 且仅在非 production 环境 + TestingKey 非空 + id 完全匹配时才放行。
	// 请勿在生产环境开启。
	TestingEnabled bool `json:"testing_enabled" mapstructure:"testing_enabled" yaml:"testing_enabled"`

	// TestingKey 测试用万能验证码的 id（非答案），需配合 TestingEnabled 使用
	TestingKey string `json:"testing_key" mapstructure:"testing_key" yaml:"testing_key"`
}

// Paging 分页相关配置
type Paging struct {
	PerPage         int    `json:"per_page" mapstructure:"per_page" yaml:"per_page"`
	UrlQueryPage    string `json:"url_query_page" mapstructure:"url_query_page" yaml:"url_query_page"`
	UrlQuerySort    string `json:"url_query_sort" mapstructure:"url_query_sort" yaml:"url_query_sort"`
	UrlQueryOrder   string `json:"url_query_order" mapstructure:"url_query_order" yaml:"url_query_order"`
	UrlQueryPerPage string `json:"url_query_per_page" mapstructure:"url_query_per_page" yaml:"url_query_per_page"`
}
