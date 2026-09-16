package app

import (
	"github.com/luct2doo/goutils/config"
	"sync"
	"time"
)

type App struct {
	appCfg *config.App
	tz     *time.Location
	tzOnce sync.Once
}

func NewApp(appCfg *config.App) *App {
	return &App{
		appCfg: appCfg,
	}
}

func (a *App) IsLocal() bool {
	return a.appCfg.Env == "local"
}

func (a *App) IsProduction() bool {
	return a.appCfg.Env == "production"
}

func (a *App) IsTesting() bool {
	return a.appCfg.Env == "testing"
}

func (a *App) TimeNowInTimezone() time.Time {
	a.tzOnce.Do(func() {
		loc, err := time.LoadLocation(a.appCfg.Timezone)
		if err != nil {
			a.tz = time.UTC
		} else {
			a.tz = loc
		}
	})
	return time.Now().In(a.tz)
}

func (a *App) URL(path string) string {
	return a.appCfg.URL + path
}

func (a *App) V1URL(path string) string {
	return a.URL("/v1/" + path)
}
