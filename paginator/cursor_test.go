package paginator

import (
	"encoding/base64"
	"errors"
	"testing"
)

type cursorRow struct {
	ID        uint
	CreatedAt string
	Name      string
}

type taggedRow struct {
	ID      uint
	SortKey int `gorm:"column:sort_key;index"`
}

type baseModel struct {
	ID        uint
	CreatedAt string
}

type embeddedRow struct {
	baseModel
	Name string
}

func TestSanitizeTieBreaker(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "id"},
		{"id", "id"},
		{"users.id", "users.id"},
		{"  status  ", "status"},
		{"-", ""},
		{"id; DROP TABLE users", "id"},
		{"id, (SELECT 1)", "id"},
	}
	for _, c := range cases {
		if got := sanitizeTieBreaker(c.in); got != c.want {
			t.Errorf("sanitizeTieBreaker(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOrderClause(t *testing.T) {
	cases := []struct {
		name string
		p    Paginator
		want string
	}{
		{"默认追加 id", Paginator{Sort: "created_at", Order: "desc", TieBreaker: "id"}, "created_at desc, id desc"},
		{"asc 方向一致", Paginator{Sort: "created_at", Order: "asc", TieBreaker: "id"}, "created_at asc, id asc"},
		{"主排序已是主键", Paginator{Sort: "id", Order: "desc", TieBreaker: "id"}, "id desc"},
		{"带表前缀同一列", Paginator{Sort: "users.id", Order: "desc", TieBreaker: "id"}, "users.id desc"},
		{"显式关闭", Paginator{Sort: "created_at", Order: "desc", TieBreaker: ""}, "created_at desc"},
		{"两列相同时不重复", Paginator{Sort: "created_at", Order: "desc", TieBreaker: "created_at"}, "created_at desc"},
	}
	for _, c := range cases {
		if got := c.p.orderClause(); got != c.want {
			t.Errorf("%s: orderClause() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBuildCursorCondition(t *testing.T) {
	cases := []struct {
		sort, tie, order, want string
	}{
		{"created_at", "id", "desc", "((created_at < ?) OR (created_at = ? AND id < ?))"},
		{"created_at", "id", "asc", "((created_at > ?) OR (created_at = ? AND id > ?))"},
		{"id", "id", "desc", "(id < ?)"},
		{"created_at", "", "desc", "(created_at < ?)"},
	}
	for _, c := range cases {
		if got := buildCursorCondition(c.sort, c.tie, c.order); got != c.want {
			t.Errorf("buildCursorCondition(%q,%q,%q) = %q, want %q", c.sort, c.tie, c.order, got, c.want)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	if s, err := EncodeCursor(nil); err != nil || s != "" {
		t.Fatalf("EncodeCursor(nil) = %q, %v", s, err)
	}
	if c, err := ParseCursor(""); c != nil || err != nil {
		t.Fatalf("ParseCursor(\"\") = %v, %v", c, err)
	}

	want := &Cursor{Sort: int64(9007199254740993), Tie: "u-1"} // 超过 float64 精度，验证不丢位
	s, err := EncodeCursor(want)
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	got, err := ParseCursor(s)
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	if got.Sort != want.Sort || got.Tie != want.Tie {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}

	if _, err := ParseCursor("!!!not-base64!!!"); !errors.Is(err, ErrCursorMalformed) {
		t.Errorf("非法 base64 应返回 ErrCursorMalformed，实际 %v", err)
	}
	noSort := base64.RawURLEncoding.EncodeToString([]byte(`{"t":1}`))
	if _, err := ParseCursor(noSort); !errors.Is(err, ErrCursorMalformed) {
		t.Errorf("缺少主排序值应返回 ErrCursorMalformed，实际 %v", err)
	}
}

func TestDefaultCursorExtractor(t *testing.T) {
	ex := DefaultCursorExtractor("created_at", "id")

	s, tie := ex(embeddedRow{baseModel: baseModel{ID: 7, CreatedAt: "2026-01-01"}})
	if s != "2026-01-01" || tie != uint(7) {
		t.Errorf("嵌入字段提取 = (%v, %v), want (\"2026-01-01\", uint(7))", s, tie)
	}

	// 指针行 + gorm column tag
	s2, tie2 := DefaultCursorExtractor("sort_key", "id")(&taggedRow{ID: 9, SortKey: 42})
	if s2 != 42 || tie2 != uint(9) {
		t.Errorf("tag/指针提取 = (%v, %v), want (42, uint(9))", s2, tie2)
	}

	// nil 行不应 panic
	if s3, tie3 := ex((*cursorRow)(nil)); s3 != nil || tie3 != nil {
		t.Errorf("nil 行应返回 (nil, nil)，实际 (%v, %v)", s3, tie3)
	}
}

func TestFinishCursorPage(t *testing.T) {
	ex := DefaultCursorExtractor("created_at", "id")

	// 多取一条：裁剪回 PerPage，HasMore=true，游标取裁剪后的最后一条
	rows := []cursorRow{
		{ID: 1, CreatedAt: "2026-01-03"},
		{ID: 2, CreatedAt: "2026-01-02"},
		{ID: 3, CreatedAt: "2026-01-01"}, // 多取的第 3 条
	}
	hasMore, next := finishCursorPage(&rows, 2, ex)
	if !hasMore {
		t.Error("HasMore 应为 true")
	}
	if len(rows) != 2 {
		t.Errorf("切片未裁剪回 PerPage，len = %d", len(rows))
	}
	c, err := ParseCursor(next)
	if err != nil {
		t.Fatalf("ParseCursor(next): %v", err)
	}
	if c.Sort != "2026-01-02" || c.Tie != int64(2) {
		t.Errorf("NextCursor = %+v, want Sort=2026-01-02 Tie=2", c)
	}

	// 不满一页：HasMore=false，仍生成游标
	one := []cursorRow{{ID: 5, CreatedAt: "2026-01-01"}}
	hasMore, next = finishCursorPage(&one, 2, ex)
	if hasMore || next == "" {
		t.Errorf("未满一页: HasMore=%v NextCursor=%q", hasMore, next)
	}

	// 非切片输入不应 panic
	notSlice := struct{}{}
	if h, n := finishCursorPage(&notSlice, 2, ex); h || n != "" {
		t.Errorf("非切片输入应返回 (false, \"\")，实际 (%v, %q)", h, n)
	}
}

// 游标模式不应执行 COUNT(*)：query 故意留 nil，一旦走到 getTotalCount 就会 panic。
// 同时 TotalCount / TotalPage / Page / Offset 必须保持零值，与 Paging 注释一致。
func TestInitPropertiesSkipsCountInCursorMode(t *testing.T) {
	p := &Paginator{}
	p.initProperties(Param{
		Page:    3,
		PerPage: 20,
		Sort:    "created_at",
		Order:   "asc",
		Cursor:  &Cursor{Sort: "2026-01-02"},
	})

	if p.TotalCount != 0 || p.TotalPage != 0 || p.Page != 0 || p.Offset != 0 {
		t.Fatalf("游标模式应跳过计数与页码: TotalCount=%d TotalPage=%d Page=%d Offset=%d",
			p.TotalCount, p.TotalPage, p.Page, p.Offset)
	}
	if p.PerPage != 20 || p.Sort != "created_at" || p.Order != "asc" || p.TieBreaker != "id" {
		t.Fatalf("游标模式仍需初始化查询属性: %+v", p)
	}
}

// 时间类型主键/排序字段走 JSON 往返后是字符串，ParseCursor 不会还原成 time.Time，
// 业务侧必须自己转回 time.Time 才能和数据库列比较。这里固化这一契约。
func TestCursorTimeValueRoundTripStaysString(t *testing.T) {
	ts := "2026-11-16T12:00:00.123456789+08:00"
	enc, err := EncodeCursor(&Cursor{Sort: ts, Tie: int64(7)})
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	c, err := ParseCursor(enc)
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	if s, ok := c.Sort.(string); !ok || s != ts {
		t.Fatalf("Sort 应保持字符串形态，实际 %T %v", c.Sort, c.Sort)
	}
}
