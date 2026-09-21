// Package paginator 处理分页逻辑
package paginator

import (
	"math"

	"github.com/luct2doo/goutils/config"
	"github.com/luct2doo/goutils/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
	PerPage int    // 请求的每页条数
	Sort    string // 排序字段 (如 "id")
	Order   string // 排序方向 ("asc" 或 "desc")
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

	// 排序顺序
	p.Order = param.Order
	if p.Order == "" {
		p.Order = "desc" // 默认降序
	}

	// 排序字段
	p.Sort = param.Sort
	if p.Sort == "" {
		p.Sort = "id" // 默认按 ID 排序
	}

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
