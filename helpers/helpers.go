package helpers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cast"
)

// IPInfo 是从 API 解析出的数据结构
type IPInfo struct {
	Query      string `json:"query"`
	Country    string `json:"country"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
	Status     string `json:"status"` // 如果是 "fail" 表示失败
}

// QueryParams 是一个示例性的查询条件载体，配合 BuildSearchMap 使用。
//
// 字段的 query 标签即最终的查询键名；实际项目中按需增减字段即可
// （BuildSearchMap 通过反射遍历所有带 query 标签的非空字符串字段）。
type QueryParams struct {
	// 时间范围查询
	StartTime string `query:"start_time"` // 开始时间
	EndTime   string `query:"end_time"`   // 结束时间

	// 状态查询
	Status string `query:"status"` // 状态
}

// Empty 类似于 PHP 的 empty() 函数
func Empty(val any) bool {
	if val == nil {
		return true
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.String, reflect.Array:
		return v.Len() == 0
	case reflect.Map, reflect.Slice:
		return v.Len() == 0 || v.IsNil()
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	default:
		return reflect.DeepEqual(val, reflect.Zero(v.Type()).Interface())
	}
}

// MicrosecondsStr 将 time.Duration 类型（nano seconds 为单位）
// 输出为小数点后 3 位的 ms （microsecond 毫秒，千分之一秒）
func MicrosecondsStr(elapsed time.Duration) string {
	return strconv.FormatFloat(float64(elapsed.Nanoseconds())/1e6, 'f', 3, 64) + "ms"
}

// RandomNumber 生成长度为 length 的随机数字字符串
// 确保第一位不为0，避免生成以0开头的数字字符串
// 参数:
//   - length: 生成字符串的长度，必须大于0
//
// 返回值:
//   - string: 生成的随机数字字符串，第一位为1-9，其余位为0-9
//
// 使用方法:
//
//	randomNum := helpers.RandomNumber(6) // 生成6位随机数字，如 "123456"
func RandomNumber(length int) string {
	if length <= 0 {
		return ""
	}

	// 第一位使用1-9的字符表（不包含0）
	firstDigitTable := [...]byte{'1', '2', '3', '4', '5', '6', '7', '8', '9'}
	// 其余位使用0-9的字符表
	allDigitTable := [...]byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}

	b := make([]byte, length)
	n, err := io.ReadAtLeast(rand.Reader, b, length)
	if n != length {
		panic(err)
	}

	// 第一位使用不包含0的字符表
	b[0] = firstDigitTable[int(b[0])%len(firstDigitTable)]

	// 其余位使用包含0的字符表
	for i := 1; i < length; i++ {
		b[i] = allDigitTable[int(b[i])%len(allDigitTable)]
	}

	return string(b)
}

// FirstElement 安全地获取 args[0]，避免 panic: runtime error: index out of range
func FirstElement(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// RandomString 生成长度为 length 的随机字符串（仅大小写字母）。
//
// 使用 crypto/rand，适用于生成上传文件名等对不可预测性有要求的场景。
// length <= 0 或随机源读取失败时返回空字符串。
func RandomString(length int) string {
	if length <= 0 {
		return ""
	}

	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

// DefaultIPAPIBaseURL 是 IPAPIProvider 的默认端点。
//
// 注意：ip-api.com 的免费额度只支持明文 HTTP，本文档不推荐在生产环境使用明文传输；
// 生产环境请通过 NewIPAPIProvider 注入自建 HTTPS 代理或改用其他服务。
const DefaultIPAPIBaseURL = "http://ip-api.com"

// IPAPIProvider 通过 ip-api.com 风格的接口查询 IP 归属地。
//
// 做成结构体而非包级函数，是为了让调用方可以替换端点与 http.Client（超时、代理、HTTPS）。
type IPAPIProvider struct {
	BaseURL string       // 形如 "http://ip-api.com"，结尾斜杠会被去掉
	Client  *http.Client // 为 nil 时使用带 5s 超时的默认 client
}

// NewIPAPIProvider 创建一个 IPAPIProvider。
//   - baseURL 为空时使用 DefaultIPAPIBaseURL
//   - client 为 nil 时使用 &http.Client{Timeout: 5 * time.Second}
func NewIPAPIProvider(baseURL string, client *http.Client) *IPAPIProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultIPAPIBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &IPAPIProvider{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  client,
	}
}

// Lookup 查询 ip 对应的中文地址，任何失败都返回「未知位置」。
func (p *IPAPIProvider) Lookup(ip string) string {
	// 判断是否是本地回环地址
	if ip == "127.0.0.1" || ip == "::1" {
		return "本地登录"
	}

	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = DefaultIPAPIBaseURL
	}

	url := fmt.Sprintf("%s/json/%s?lang=zh-CN", strings.TrimRight(baseURL, "/"), ip)

	resp, err := client.Get(url)
	if err != nil {
		return "未知位置"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "未知位置"
	}

	var info IPInfo
	err = json.Unmarshal(body, &info)
	if err != nil {
		return "未知位置"
	}

	if info.Status == "fail" {
		return "未知位置"
	}

	var parts []string
	if info.Country != "" {
		parts = append(parts, info.Country)
	}
	if info.RegionName != "" {
		parts = append(parts, info.RegionName)
	}
	if info.City != "" && info.City != info.RegionName {
		parts = append(parts, info.City)
	}

	if len(parts) == 0 {
		return "未知位置"
	}

	return strings.Join(parts, " ")
}

// defaultIPAPIProvider 供包级函数 GetLocationFromIP 使用
var defaultIPAPIProvider = NewIPAPIProvider(DefaultIPAPIBaseURL, nil)

// GetLocationFromIP 根据 IP 查询并返回中文格式地址。
//
// 使用默认 Provider（明文 HTTP 的公共端点）。生产环境建议改用
// NewIPAPIProvider 注入 HTTPS 端点后调用 Lookup。
func GetLocationFromIP(ip string) string {
	return defaultIPAPIProvider.Lookup(ip)
}

// BuildSearchMap 根据QueryParams结构体构建搜索条件map
// 参数:
//   - params: QueryParams结构体实例，包含各种查询条件
//
// 返回值:
//   - map[string]string: 包含非空查询条件的map，可直接用于数据库查询
//
// 使用方法:
//
//	params := helpers.QueryParams{StartTime: "2023-01-01", EndTime: "2023-12-31"}
//	searchMap := helpers.BuildSearchMap(params)
//
// searchMap 将包含 {"start_time": "2023-01-01", "end_time": "2023-12-31"}
func BuildSearchMap(params QueryParams) map[string]string {
	search := make(map[string]string)

	// 使用反射获取结构体的值和类型信息
	v := reflect.ValueOf(params)
	t := reflect.TypeOf(params)

	// 遍历结构体的所有字段
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)     // 获取字段值
		fieldType := t.Field(i) // 获取字段类型信息

		// 获取 query 标签, 如果没有则路过
		queryTag := fieldType.Tag.Get("query")
		if queryTag == "" {
			continue
		}

		// 检查字段是否为字符串类型且不为空
		if field.Kind() == reflect.String {
			value := field.String()
			if value != "" {
				search[queryTag] = value
			}
		}
	}
	return search
}

// BuildSearchMapFromGin 从Gin Context中提取查询参数并构建搜索条件map
// 参数:
//   - c: gin.Context实例
//   - queryFields: 需要提取的查询字段名称列表
//
// 返回值:
//   - map[string]string: 包含非空查询条件的map
//
// 使用方法:
//
//	searchMap := helpers.BuildSearchMapFromGin(c, []string{"start_time", "end_time", "status"})
func BuildSearchMapFromGin(c any, queryFields []string) map[string]string {
	search := make(map[string]string)

	// 这里需要导入gin包，为了避免循环依赖，我们使用any
	// 在实际使用时，调用者需要确保传入的是*gin.Context
	// 由于helpers包不应该依赖gin，这个函数作为备选方案

	// 使用反射调用DefaultQuery方法
	v := reflect.ValueOf(c)
	defaultQueryMethod := v.MethodByName("DefaultQuery")

	if !defaultQueryMethod.IsValid() {
		return search
	}

	for _, field := range queryFields {
		// 调用c.DefaultQuery(field, "")
		args := []reflect.Value{
			reflect.ValueOf(field),
			reflect.ValueOf(""),
		}
		result := defaultQueryMethod.Call(args)

		if len(result) > 0 {
			value := result[0].String()
			if value != "" {
				search[field] = value
			}
		}
	}

	return search
}

// GenerateSecureDigit 生成10位的随机数字字符串
func GenerateSecureDigit(length int) string {
	min := new(big.Int)
	min.SetString("1000000000", length)
	max := new(big.Int)
	max.SetString("9999999999", length)

	n, err := rand.Int(rand.Reader, max.Sub(max, min).Add(min, big.NewInt(1)))
	if err != nil {
		return ""
	}

	return n.String()
}

func TimeNow() string {
	return time.Now().Format(time.DateTime)
}

// GetLastSegment 获取分隔字符串的最后一段
// 功能描述：从以指定分隔符分隔的字符串中提取最后一段内容
// 参数说明：
//   - input: 输入字符串
//   - separator: 分隔符
//
// 返回值：string - 最后一段内容
// 使用方法：helpers.GetLastSegment("a|b|c", "|")
func GetLastSegment(input, separator string) string {
	if Empty(input) {
		return ""
	}

	segments := strings.Split(input, separator)
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}

// GetLastIdea 获取最后一个想法（专门用于想法字符串处理）
// 功能描述：从以"|"分隔的想法字符串中提取最后一个想法
// 参数说明：
//   - ideas: 想法字符串
//
// 返回值：string - 最后一个想法
// 使用方法：helpers.GetLastIdea("想法1|想法2|想法3")
func GetLastIdea(ideas string) string {
	return GetLastSegment(ideas, "|")
}

// contains 检查切片中是否包含某个字符串
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// addIfNotExists 如果切片中不存在该字符串，则添加

func AddIfNotExists(slice []string, item string) []string {
	if !contains(slice, item) {
		slice = append(slice, item)
	}
	return slice
}

func GetUnixTime() int64 {
	return time.Now().Unix()
}

func TrimStruct(s any) {
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return
	}
	ve := v.Elem()
	if ve.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < ve.NumField(); i++ {
		field := ve.Field(i)

		// 修剪直接的 string 字段
		if field.Kind() == reflect.String && field.CanSet() {
			str := field.String()
			field.SetString(strings.TrimSpace(str))
			continue
		}

		// 修剪 *string 字段的值
		if field.Kind() == reflect.Pointer && !field.IsNil() && field.Elem().Kind() == reflect.String {
			elem := field.Elem()
			if elem.CanSet() {
				elem.SetString(strings.TrimSpace(elem.String()))
			}
			continue
		}

		// 递归处理嵌套结构体
		if field.Kind() == reflect.Struct && field.CanAddr() {
			TrimStruct(field.Addr().Interface())
			continue
		}
	}
}

// UnixToString 将 int64 的 Unix 时间戳格式化为 "2006-01-02 15:04:05" 字符串
// 功能描述：
//   - 后端通常在数据库中使用 Unix 时间戳（int64）存储时间，因为它无视时区，性能好且方便比较。
//   - 但在某些特定场景下，如果非要后端返回格式化后的字符串给前端，可以使用此函数。
//   - （注意：现代 Web 开发中，通常推荐后端直接返回 int64 或 RFC3339 格式，由前端根据用户本地时区去格式化）
//
// 参数说明：
//   - timestamp: int64 - Unix 时间戳（秒级）
//
// 返回值：
//   - string - 格式化后的时间字符串，如果传入 0 则返回空字符串
func UnixToString(timestamp int64) string {
	if timestamp == 0 {
		return ""
	}
	return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
}

// StringToUnix 将 "2006-01-02 15:04:05" 格式的字符串解析为 Unix 时间戳
// 功能描述：
//   - 处理反向转换：将前端传来的本地时间字符串转回后端统一使用的 Unix 时间戳
//
// 参数说明：
//   - timeStr: string - 格式化后的时间字符串
//
// 返回值：
//   - int64 - Unix 时间戳，如果解析失败或为空则返回 0
func StringToUnix(timeStr string) int64 {
	if timeStr == "" {
		return 0
	}
	// 注意：这里的 Parse 默认使用 UTC 时区。
	// 如果前端传过来的是北京时间（没有带时区后缀），更严谨的做法是使用 time.ParseInLocation
	// t, err := time.ParseInLocation("2006-01-02 15:04:05", timeStr, time.Local)
	t, err := time.Parse("2006-01-02 15:04:05", timeStr)
	if err != nil {
		return 0
	}
	return t.Unix()
}

// Now 统一返回 UTC 时间，彻底屏蔽环境 TZ 差异
func Now() time.Time {
	return time.Now().UTC()
}

// ParseRFC3339 安全解析前端/三方传入的 ISO8601 时间
func ParseRFC3339(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// InCST 仅在输出给前端/日志时调用
var CST = time.FixedZone("CST", 8*3600)

func ToCST(t time.Time) time.Time {
	return t.In(CST)
}

// BuildPaginatorParam 从任意包含分页字段的请求结构体构建 paginator.Param
// 功能描述：
//
//	通过反射或接口转换，将 Handler 层接收到的各类请求结构体（如 requests.PaginationRequest）
//	快速转换为底层业务和数据层通用的 paginator.Param，减少重复的强转代码。
//
// 使用方法：
//
//	param := helpers.BuildPaginatorParam(req)
func BuildPaginatorParam(req any) (int, int, string, string) {
	page, perPage, sort, order := 0, 0, "", ""

	v := reflect.ValueOf(req)
	// 如果是指针，解引用
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return page, perPage, sort, order
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return page, perPage, sort, order
	}

	// 提取 Page
	if pageField := v.FieldByName("Page"); pageField.IsValid() {
		page = cast.ToInt(pageField.Interface())
	}

	// 提取 PerPage
	if perPageField := v.FieldByName("PerPage"); perPageField.IsValid() {
		perPage = cast.ToInt(perPageField.Interface())
	}

	// 提取 Sort
	if sortField := v.FieldByName("Sort"); sortField.IsValid() {
		sort = cast.ToString(sortField.Interface())
	}

	// 提取 Order
	if orderField := v.FieldByName("Order"); orderField.IsValid() {
		order = cast.ToString(orderField.Interface())
	}

	return page, perPage, sort, order
}

func CheckPort(port string) error {
	// 尝试连接 IPv4 和 IPv6，任一能连上说明端口已被占用
	for _, addr := range []string{"127.0.0.1" + port, "[::1]" + port} {
		conn, err := net.DialTimeout("tcp", addr, time.Millisecond*100)
		if err == nil {
			conn.Close()
			return fmt.Errorf("端口已被其他进程占用 (%s)", addr)
		}
	}
	// 都连不上，再尝试监听，确认端口可用
	ln, err := net.Listen("tcp", port)
	if err != nil {
		return err
	}
	return ln.Close()
}

func CountStructFields(structure any) int {
	t := reflect.TypeOf(structure)
	if t.Kind() == reflect.Struct {
		return t.NumField()
	}
	return 0
}

func ToTime(unixTime string) time.Time {
	// ================= 步骤 1：处理特殊空值 =================
	// 为什么需要这一步？
	// 在业务中，空字符串或 "0" 通常代表“未设置”或“默认值”。
	// 提前拦截并返回零值 time.Time{}，可以避免后续转换报错，也方便调用方通过 .IsZero() 判断。
	if unixTime == "" || unixTime == "0" {
		return time.Time{}
	}

	// ================= 步骤 2：字符串转 int64 =================
	// 为什么使用 strconv.ParseInt？
	// 因为 time.Unix() 需要接收 int64 类型的参数，而我们的输入是 string。
	// 参数说明：
	//   - unixTime: 要解析的字符串
	//   - 10: 表示字符串是十进制数字（如果是 16 就是十六进制）
	//   - 64: 表示结果需要是 64 位整数（int64），防止时间戳过大导致 32 位溢出
	ts, err := strconv.ParseInt(unixTime, 10, 64)
	if err != nil {
		// 如果转换失败（比如传入了 "abc" 或 "12.34"），说明格式不合法。
		// 这里选择静默失败并返回零值。
		// 提示：如果在实际业务中你需要知道具体报了什么错，可以把函数签名改为返回 (time.Time, error)。
		return time.Time{}
	}

	// ================= 步骤 3：自动判断精度并转换 =================
	// 为什么需要判断？
	// 10位数字（如 1694502400）通常是秒级时间戳。
	// 13位数字（如 1694502400000）通常是毫秒级时间戳。
	// 253402271999 是公元 9999-12-31 23:59:59 的秒级时间戳最大值。
	// 如果 ts 大于这个值，说明它大概率不是秒级，而是毫秒级。
	if ts > 253402271999 {
		// 处理毫秒级时间戳
		// time.Unix(sec, nsec) 的第一个参数是“秒”，第二个参数是“纳秒”。
		// 所以我们需要把毫秒拆分成 秒 和 纳秒：
		sec := ts / 1000              // 整除 1000 得到完整的秒数
		nsec := (ts % 1000) * 1000000 // 取余数得到剩余毫秒，再乘 1,000,000 转换为纳秒 (1毫秒 = 1,000,000纳秒)
		return time.Unix(sec, nsec)
	}

	// ================= 步骤 4：处理秒级时间戳 =================
	// 如果数值在合理秒级范围内，直接作为秒传入，纳秒部分传 0 即可
	return time.Unix(ts, 0)
}
