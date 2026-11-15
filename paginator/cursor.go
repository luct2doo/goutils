// Package paginator 的游标（keyset）分页部分。
//
// 与 OFFSET 分页的区别：
//   - WHERE 用「上一页最后一条的排序值」定位，不再 OFFSET 扫描，深翻页代价恒定；
//   - 不再执行 COUNT(*)，因此 Paging 里没有 TotalCount / TotalPage / CurrentPage；
//   - 不能跳到第 N 页，只能一页页往后翻，用 NextCursor + HasMore 驱动。
package paginator

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/luct2doo/goutils/logger"

	"gorm.io/gorm/clause"
)

var ErrCursorMalformed = errors.New("分页游标格式有误")

// Cursor 记录上一页最后一条的 (主排序字段值, tie-breaker 值)。
// Sort / Tie 会被 JSON 序列化，只放可比较的标量：数字、字符串、格式化后的时间字符串。
// 不要往里放敏感数据或用于鉴权的字段。
type Cursor struct {
	Sort any `json:"s"`
	Tie  any `json:"t"`
}

// EncodeCursor 把游标编码成可放进 URL / 响应体的字符串（base64url + JSON）。
// 不做签名：游标只表示翻页位置，不能用来鉴权。nil 返回空串，表示没有下一页。
func EncodeCursor(c *Cursor) (string, error) {
	if c == nil {
		return "", nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ParseCursor 解码游标。空串返回 (nil, nil)，表示第一页（此时走 OFFSET 分页）。
// 数字用 json.Number 解码后还原成 int64 / float64，避免大整数主键丢精度。
//
// 若主排序字段是时间类型，JSON 里必然是字符串，业务侧需要按自己的列格式转回
// time.Time 再赋给 c.Sort，或者直接提供自定义 CursorExtractor 把值规范成可比较的字符串。
func ParseCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, ErrCursorMalformed
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var c Cursor
	if err := dec.Decode(&c); err != nil {
		return nil, ErrCursorMalformed
	}
	c.Sort = normalizeCursorValue(c.Sort)
	c.Tie = normalizeCursorValue(c.Tie)
	if c.Sort == nil {
		return nil, ErrCursorMalformed
	}
	return &c, nil
}

func normalizeCursorValue(v any) any {
	n, ok := v.(json.Number)
	if !ok {
		return v
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return f
	}
	return n.String()
}

// CursorExtractor 从结果集的一行里取出 (主排序值, tie-breaker 值)。
// 返回 nil 的主排序值表示无法生成游标，此时 NextCursor 留空。
type CursorExtractor func(row any) (sortValue, tieValue any)

// DefaultCursorExtractor 按列名反射匹配结构体字段，开箱即用。
// 匹配规则：导出字段名的 snake_case（CreatedAt → created_at、ID → id），
// 或 `gorm:"column:xxx"` 里显式声明的列名；支持匿名嵌入字段（如 gorm.Model）。
//
// 列类型复杂（时间、自定义类型）时请传自定义 CursorExtractor，库不猜列类型。
func DefaultCursorExtractor(sortField, tieField string) CursorExtractor {
	return func(row any) (any, any) {
		v := reflect.ValueOf(row)
		for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
			if v.IsNil() {
				return nil, nil
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return nil, nil
		}
		return fieldByColumn(v, baseColumn(sortField)), fieldByColumn(v, baseColumn(tieField))
	}
}

// baseColumn 去掉 table.column 的表前缀。
func baseColumn(col string) string {
	if i := strings.LastIndex(col, "."); i >= 0 {
		col = col[i+1:]
	}
	return strings.TrimSpace(col)
}

func fieldByColumn(v reflect.Value, col string) any {
	if col == "" {
		return nil
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)

		// 匿名嵌入字段（gorm.Model 等）递归找
		if f.Anonymous {
			p := fv
			for p.Kind() == reflect.Ptr {
				if p.IsNil() {
					break
				}
				p = p.Elem()
			}
			if p.Kind() == reflect.Struct {
				if got := fieldByColumn(p, col); got != nil {
					return got
				}
			}
		}

		if f.PkgPath != "" { // 未导出字段
			continue
		}
		if strcase.ToSnake(f.Name) == col || gormColumn(f) == col {
			return fv.Interface()
		}
	}
	return nil
}

func gormColumn(f reflect.StructField) string {
	for _, part := range strings.Split(f.Tag.Get("gorm"), ";") {
		if strings.HasPrefix(part, "column:") {
			return strings.TrimSpace(strings.TrimPrefix(part, "column:"))
		}
	}
	return ""
}

// hasTieBreaker 判断这次查询是否真的需要追加第二排序键。
// 主排序本身是主键（或两者指向同一列）时追加没有意义，反而生成冗余 SQL。
func hasTieBreaker(sort, tie string) bool {
	if tie == "" || tie == sort {
		return false
	}
	return baseColumn(tie) != baseColumn(sort)
}

// buildCursorCondition 生成 keyset 分页的 WHERE 片段。
//
// 用 OR 展开而不是 (a, b) < (?, ?) 行值比较：SQLite 对行值比较支持有限，
// 展开式在 PostgreSQL / MySQL / SQLite 上语义一致且能走联合索引。
// 外层括号不可省——查询上通常已有其它 WHERE 条件，裸 OR 会改变 AND 的优先级。
//
// sort / tie 必须已经过 sanitizeSort / sanitizeTieBreaker 的字形校验后才传进来。
// 排序字段允许 NULL 时结果会漏行，请用 NOT NULL 列或在业务侧规范化。
func buildCursorCondition(sort, tie, order string) string {
	cmp := "<"
	if order == "asc" {
		cmp = ">"
	}
	if !hasTieBreaker(sort, tie) {
		return fmt.Sprintf("(%s %s ?)", sort, cmp)
	}
	return fmt.Sprintf("((%s %s ?) OR (%s = ? AND %s %s ?))", sort, cmp, sort, tie, cmp)
}

// paginateByCursor 执行游标分页。多取一条用于精确判断 HasMore，随后把切片裁剪回 PerPage。
func (p *Paginator) paginateByCursor(data any, param Param) *Paging {
	cursor := param.Cursor
	if cursor == nil || cursor.Sort == nil {
		return &Paging{}
	}

	cond := buildCursorCondition(p.Sort, p.TieBreaker, p.Order)
	vars := []any{cursor.Sort}
	if hasTieBreaker(p.Sort, p.TieBreaker) {
		vars = []any{cursor.Sort, cursor.Sort, cursor.Tie}
	}

	err := p.query.
		Preload(clause.Associations).
		Where(cond, vars...).
		Order(p.orderClause()).
		Limit(p.PerPage + 1).
		Find(data).Error
	if err != nil {
		logger.LogIf(err)
		return &Paging{}
	}

	paging := &Paging{PerPage: p.PerPage}
	extractor := param.CursorExtractor
	if extractor == nil {
		extractor = DefaultCursorExtractor(p.Sort, p.TieBreaker)
	}
	paging.HasMore, paging.NextCursor = finishCursorPage(data, p.PerPage, extractor)
	return paging
}

// finishCursorPage 把「多取一条」的结果裁剪回 PerPage，并用最后一条生成下一页游标。
// 抽成纯函数，便于脱离数据库验证裁剪与游标取值。
func finishCursorPage(data any, perPage int, extractor CursorExtractor) (bool, string) {
	hasMore := false
	if v := sliceValue(data); v.IsValid() && v.CanSet() && v.Len() > perPage {
		hasMore = true
		v.SetLen(perPage)
	}

	last := sliceLast(data)
	if last == nil || extractor == nil {
		return hasMore, ""
	}
	sortValue, tieValue := extractor(last)
	if sortValue == nil {
		return hasMore, ""
	}
	next, err := EncodeCursor(&Cursor{Sort: sortValue, Tie: tieValue})
	if err != nil {
		logger.LogIf(err)
		return hasMore, ""
	}
	return hasMore, next
}

// sliceValue 取 data 指向的切片值；data 不是切片时返回无效 Value。
func sliceValue(data any) reflect.Value {
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice {
		return reflect.Value{}
	}
	return v
}

func sliceLast(data any) any {
	v := sliceValue(data)
	if !v.IsValid() || v.Len() == 0 {
		return nil
	}
	return v.Index(v.Len() - 1).Interface()
}
