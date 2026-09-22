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

	query *gorm.DB // db query 句柄
	cfg   *config.Paging
}

func NewPaginator(cfg *config.Paging) *Paginator {
	return &Paginator{
		cfg: cfg,
	}
}

// Paginate 分页核心方法
// db —— GORM 查询句柄，用以查询数据集和获取数据总数
// data —— 模型数组指针，传址获取数据
// param —— 从外部（如 Handler 层）传入的分页参数
func (p *Paginator) Paginate(db *gorm.DB, data any, param Param) *Paging {
	// 每次调用创建新的实例，避免并发污染，但保留注入的 cfg
	instance := &Paginator{
		query: db,
		cfg:   p.cfg,
	}

	instance.initProperties(param)

	// 查询数据库
	err := instance.query.Preload(clause.Associations). // 读取关联
								Order(instance.Sort + " " + instance.Order). // 排序
								Limit(instance.PerPage).
								Offset(instance.Offset).
								Find(data).
								Error

	// 数据库出错
	if err != nil {
		logger.LogIf(err)
		return &Paging{}
	}

	return &Paging{
		CurrentPage: instance.Page,
		PerPage:     instance.PerPage,
		TotalPage:   instance.TotalPage,
		TotalCount:  instance.TotalCount,
	}
}

// 初始化分页必须用到的属性，基于这些属性查询数据库
func (p *Paginator) initProperties(param Param) {
	// 获取每页数量
	p.PerPage = p.getPerPage(param.PerPage)

	// 排序顺序：只接受 asc / desc，其余一律回退默认值
	p.Order = normalizeOrder(param.Order)

	// 排序字段：字形校验 + 可选白名单，任何不合法输入都回退为 id。
	// 这里是安全边界——Sort/Order 会被拼进 Order() 的 SQL 片段。
	p.Sort = sanitizeSort(param.Sort, param.AllowedSorts)

	// 核心修复：必须先计算总条数和总页数，再计算当前页和 Offset
	p.TotalCount = p.getTotalCount()
	p.TotalPage = p.getTotalPage()

	// getCurrentPage 依赖 TotalPage 进行边界检查
	p.Page = p.getCurrentPage(param.Page)

	// 计算 Offset
	if p.Page > 0 {
		p.Offset = (p.Page - 1) * p.PerPage
	} else {
		p.Offset = 0
	}
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
	// 优先使用请求中的 per_page 参数
	if reqPerPage > 0 {
		return reqPerPage
	}

	// 其次使用配置中的默认值
	if p.cfg != nil && p.cfg.PerPage > 0 {
		return p.cfg.PerPage
	}

	// 最终兜底默认值
	return 10
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
func (p *Paginator) getTotalCount() int64 {
	var count int64
	if err := p.query.Count(&count).Error; err != nil {
		return 0
	}
	return count
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
