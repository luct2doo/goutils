package cache

import (
	"testing"

	"github.com/luct2doo/goutils/config"
)

func TestProvideStoreUsesConfiguredPrefix(t *testing.T) {
	appCfg := &config.App{Name: "demo"}

	store := ProvideStore(&config.Cache{Prefix: "rugao"}, nil, appCfg).(*RedisStore)
	if store.KeyPrefix != "rugao:cache:" {
		t.Errorf("KeyPrefix = %q, want %q", store.KeyPrefix, "rugao:cache:")
	}

	// 未配置 prefix 时回退应用名
	store = ProvideStore(&config.Cache{}, nil, appCfg).(*RedisStore)
	if store.KeyPrefix != "demo:cache:" {
		t.Errorf("KeyPrefix = %q, want %q", store.KeyPrefix, "demo:cache:")
	}

	store = ProvideStore(nil, nil, appCfg).(*RedisStore)
	if store.KeyPrefix != "demo:cache:" {
		t.Errorf("KeyPrefix = %q, want %q", store.KeyPrefix, "demo:cache:")
	}
}
