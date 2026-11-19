package paginator

import (
	"testing"

	"github.com/luct2doo/goutils/config"
)

func TestGetPerPageClampsToMax(t *testing.T) {
	p := NewPaginator(&config.Paging{PerPage: 20})

	if got := p.getPerPage(0); got != 20 {
		t.Errorf("getPerPage(0) = %d, want 20（配置默认值）", got)
	}
	if got := p.getPerPage(10_000_000); got != maxPerPage {
		t.Errorf("getPerPage(超大值) = %d, want %d", got, maxPerPage)
	}
	if got := p.getPerPage(-1); got != 20 {
		t.Errorf("getPerPage(-1) = %d, want 20", got)
	}

	// 未注入配置时回退内置默认值
	if got := NewPaginator(nil).getPerPage(0); got != defaultPerPage {
		t.Errorf("getPerPage(0) 无配置 = %d, want %d", got, defaultPerPage)
	}
}
