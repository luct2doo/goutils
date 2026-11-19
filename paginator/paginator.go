// Package paginator 处理分页逻辑
package paginator

import (
	"math"
	"regexp"
	"strings"

	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultSortField = "id"   // 默认排序字段
	defaultSortOrder = "desc" // 默认排序方向

	defaultPerPage = 10  // 未配置时的默认每页条数
	maxPerPage     = 100 // PerPage 上限，防止客户端传超大值把数据库打爆
)

// sortKeyPattern 排序字段的字形校验：column 或 table.column。
//
// 排序字段无法通过 GORM 的参数绑定传参（Order 接收的是 SQL 片段），
// 因此必须在这里做白名单式校验，否则就是 SQL 注入入口。
var sortKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)?$`)

// Paging 分页响应数据
type Paging struct {
	CurrentPage int   `json:"current_page"` // 当前页
	PerPage     int   `json:"per_page"`     // 每页条数
	TotalPage   int   `json:"total_page"`   // 总页数
	TotalCount  int64 `json:"total_count"`  // 总条数

	// 以下两项仅游标模式有值。游标模式不执行 COUNT(*)，
	// 因此 CurrentPage / TotalPage / TotalCount 均为 0。
	NextCursor string `json:"next_cursor,omitempty"` // 下一页游标，空串表示没有更多数据
	HasMore    bool   `json:"has_more,omitempty"`    // 是否还有下一页
}

// Param 分页请求参数，由 Handler 层从 HTTP 或 gRPC 请求中提取后传入
type Param struct {
	Page    int    // 请求的页码
	PerPage int    // 每页条数
	Sort    string // 排序字段 (如 "id")
	Order   string // 排序方向 ("asc" 或 "desc")

	// AllowedSorts 可选的排序字段白名单。
	// 非空时 Sort 必须命中其中之一（区分大小写）；为空时仅做字形校验。
	// 注意：白名单里的值同样要通过字形校验，避免把注入片段当成合法候选。
	AllowedSorts []string

	// TieBreaker 第二排序键，保证 ORDER BY 唯一稳定，避免翻页重复/漏数据。
	// 零值默认 "id"，方向与 Order 一致；传 "-" 表示关闭（仅当 Sort 本身唯一时才可关闭）。
	// 字段名同样过字形校验，但不受 AllowedSorts 限制——它是内部追加的主键。
	TieBreaker string

	// Cursor 非空时切换到游标（keyset）分页：忽略 Page / Offset，也不再执行 COUNT(*)。
	// 用 ParseCursor 从请求里的 cursor 参数解码；空串会返回 nil，即回到 OFFSET 模式。
	Cursor *Cursor

	// CursorExtractor 从结果行提取游标值；为空时使用 DefaultCursorExtractor(Sort, TieBreaker)。
	// 排序字段是时间或自定义类型时，建议传自定义实现。
	CursorExtractor CursorExtractor
}

// BuildParam 从请求数据构建分页参数
// req 可以是任何包含了 Page、PerPage、Sort、Order 字段的结构体（例如 HTTP 或 gRPC 的 Request）
func BuildParam(page, perPage int, sort, order string) Param {
	return Param{
		Page:    page,
		PerPage: perPage,
		Sort:    sort,
		Order:   order,
	}
}

// Paginator 分页操作类
type Paginator struct {
	PerPage    int    // 每页条数
	Page       int    // 当前页
	Offset     int    // 数据库读取数据时 Offset 的值
	TotalCount int64  // 总条数
	TotalPage  int    // 总页数 = TotalCount/PerPage
	Sort       string // 排序规则
	Order      string // 排序顺序
	TieBreaker string // 第二排序键，保证排序唯一稳定

	query *gorm.DB // db query 句柄
	cfg   *config.Paging
}

func NewPaginator(cfg *config.Paging) *Paginator {
	return &Paginator{
		cfg: cfg,
	}
}

// Paginate 分页核心方法（兼容旧调用：查询出错时记录日志并返回空 Paging）
// db —— GORM 查询句柄，用以查询数据集和获取数据总数
// data —— 模型数组指针，传址获取数据
// param —— 从外部（如 Handler 层）传入的分页参数
//
// 需要区分「没有数据」和「查询失败」时请改用 PaginateWithError。
func (p *Paginator) Paginate(db *gorm.DB, data any, param Param) *Paging {
	paging, err := p.PaginateWithError(db, data, param)
	if err != nil {
		logger.LogIf(err)
		return &Paging{}
	}
	return paging
}

// PaginateWithError 与 Paginate 行为一致，但会把数据库错误原样返回给调用方。
func (p *Paginator) PaginateWithError(db *gorm.DB, data any, param Param) (*Paging, error) {
	// 每次调用创建新的实例，避免并发污染，但保留注入的 cfg
	instance := &Paginator{
		query: db,
		cfg:   p.cfg,
	}

	if err := instance.initProperties(param); err != nil {
		return nil, err
	}

	// 游标模式：不 OFFSET、不 COUNT(*)，靠上一页最后一条的排序值定位
	if param.Cursor != nil {
		return instance.paginateByCursor(data, param)
	}

	// 查询数据库
	err := instance.query.Preload(clause.Associations). // 读取关联
								Order(instance.orderClause()). // 主排序 + tie-breaker
								Limit(instance.PerPage).
								Offset(instance.Offset).
								Find(data).
								Error
	if err != nil {
		return nil, err
	}

	return &Paging{
		CurrentPage: instance.Page,
		PerPage:     instance.PerPage,
		TotalPage:   instance.TotalPage,
		TotalCount:  instance.TotalCount,
	}, nil
}

// 初始化分页必须用到的属性，基于这些属性查询数据库
func (p *Paginator) initProperties(param Param) error {
	// 获取每页数量
	p.PerPage = p.getPerPage(param.PerPage)

	// 排序顺序：只接受 asc / desc，其余一律回退默认值
	p.Order = normalizeOrder(param.Order)

	// 排序字段：字形校验 + 可选白名单，任何不合法输入都回退为 id。
	// 这里是安全边界——Sort/Order 会被拼进 Order() 的 SQL 片段。
	p.Sort = sanitizeSort(param.Sort, param.AllowedSorts)

	// 第二排序键：默认 id，方向跟随 Order，保证排序结果唯一稳定。
	p.TieBreaker = sanitizeTieBreaker(param.TieBreaker)

	// 游标模式到此为止：不 COUNT(*)、不算页码、不算 Offset。
	// COUNT(*) 在 keyset 分页里没有语义，而且是一次额外的全量扫描，
	// 游标模式下 TotalCount / TotalPage / CurrentPage 保持 0（与 Paging 注释一致）。
	if param.Cursor != nil {
		return nil
	}

	// 核心修复：必须先计算总条数和总页数，再计算当前页和 Offset
	totalCount, err := p.getTotalCount()
	if err != nil {
		return err
	}
	p.TotalCount = totalCount
	p.TotalPage = p.getTotalPage()

	// getCurrentPage 依赖 TotalPage 进行边界检查
	p.Page = p.getCurrentPage(param.Page)

	// 计算 Offset
	if p.Page > 0 {
		p.Offset = (p.Page - 1) * p.PerPage
	} else {
		p.Offset = 0
	}

	return nil
}

// orderClause 生成 ORDER BY 片段。只要主排序键不是唯一的，就必须追加 tie-breaker，
// 否则数据库对相同排序值的行返回顺序不确定，OFFSET 翻页会重复 / 漏数据。
// 第二排序键方向与主排序一致，这样 WHERE 的 keyset 条件才能用同一个比较符。
func (p *Paginator) orderClause() string {
	if !hasTieBreaker(p.Sort, p.TieBreaker) {
		return p.Sort + " " + p.Order
	}
	return p.Sort + " " + p.Order + ", " + p.TieBreaker + " " + p.Order
}

// sanitizeTieBreaker 校验第二排序键：非法输入回退为 "id"，"-" 表示关闭。
// 这里不套 AllowedSorts——tie-breaker 是内部追加的主键，不由请求直接指定。
func sanitizeTieBreaker(tie string) string {
	tie = strings.TrimSpace(tie)
	if tie == "-" {
		return ""
	}
	if tie == "" || !sortKeyPattern.MatchString(tie) {
		return defaultSortField
	}
	return tie
}

// sanitizeSort 校验排序字段，不合法时回退为 defaultSortField。
//
// 规则（按顺序）：
//  1. 去空格后为空 → 回退
//  2. 不满足 sortKeyPattern（column 或 table.column）→ 回退
//  3. allowed 非空且 sort 不在其中 → 回退
func sanitizeSort(sort string, allowed []string) string {
	sort = strings.TrimSpace(sort)
	if sort == "" || !sortKeyPattern.MatchString(sort) {
		return defaultSortField
	}
	if len(allowed) == 0 {
		return sort
	}
	for _, a := range allowed {
		if a == sort {
			return sort
		}
	}
	return defaultSortField
}

// normalizeOrder 校验排序方向，仅接受 asc / desc（大小写不敏感），其余回退为 desc
func normalizeOrder(order string) string {
	switch strings.ToLower(strings.TrimSpace(order)) {
	case "asc":
		return "asc"
	case "desc":
		return "desc"
	default:
		return defaultSortOrder
	}
}

func (p *Paginator) getPerPage(reqPerPage int) int {
	perPage := reqPerPage

	// 请求没传时，依次回退到配置默认值和内置默认值
	if perPage <= 0 {
		if p.cfg != nil && p.cfg.PerPage > 0 {
			perPage = p.cfg.PerPage
		} else {
			perPage = defaultPerPage
		}
	}

	// 上限保护：PerPage 会直接变成 SQL 的 LIMIT，必须钳制
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return perPage
}

// getCurrentPage 返回当前页码
func (p *Paginator) getCurrentPage(reqPage int) int {
	page := reqPage
	if page <= 0 {
		page = 1 // 默认为 1
	}

	// TotalPage 等于 0 ，意味着数据不够分页
	if p.TotalPage == 0 {
		return 0
	}

	// 请求页数大于总页数，返回总页数
	if page > p.TotalPage {
		return p.TotalPage
	}
	return page
}

// getTotalCount 返回的是数据库里的条数
func (p *Paginator) getTotalCount() (int64, error) {
	var count int64
	if err := p.query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// getTotalPage 计算总页数
func (p *Paginator) getTotalPage() int {
	if p.TotalCount == 0 {
		return 0
	}
	nums := int64(math.Ceil(float64(p.TotalCount) / float64(p.PerPage)))
	if nums == 0 {
		nums = 1
	}
	return int(nums)
}
