package helpers

import (
	"fmt"
	"reflect"
	"strings"
)

// BuildUpdatesMap 将“指针字段的请求 DTO”转换为 Updates(map[string]any)
// 功能描述:
//
//	在前端通过 HTTP PUT/PATCH 提交数据更新时，由于 Go 语言的基础类型（如 string, int, bool）存在零值（如 "", 0, false），
//	我们无法区分前端是“没有提交这个字段”还是“故意提交了零值来清空这个字段”。
//	为了解决这个问题，我们在 Request 结构体中使用指针类型（如 *string, *int, *bool）。
//	如果前端没有提交该字段，JSON 解析后指针为 nil；如果提交了，指针就会有具体的值（即使是零值）。
//	本函数的作用就是：遍历这个 Request 结构体，找出所有“非 nil 的指针字段”，
//	提取它们的实际值，并根据 tagName（通常是 json）作为键，拼装成一个 map[string]any，
//	最后这个 map 可以直接丢给 GORM 的 Updates 方法进行局部更新，避免了将未提交的字段被错误地更新为零值。
//
// 参数说明:
//   - obj: any，必须是“结构体指针”，希望被更新的字段应声明为指针类型（如 *string、*uint8、*bool）。
//   - tagName: string，提取键名的标签（常用 "json"）。
//   - allowed: []string，可选白名单，内容为标签键名。nil 表示不过滤（允许所有字段）；非 nil（含空切片）表示为白名单集合，空集合则全部拦截。该语义与 BuildDiffUpdatesMapJSON 一致。
//
// 返回值:
//   - map[string]any: 更新集合，仅包含“非 nil 指针字段”，键为去除 ",omitempty" 等修饰后的标签键。
//   - error: 参数非法或没有任何可更新字段时返回错误。
//
// 调用示例:
//
//	updates, err := BuildUpdatesMap(&req, "json", []string{"status","send_log"})
//	db.Model(&Model{}).Where(...).Updates(updates)
func BuildUpdatesMap(obj any, tagName string, allowed []string) (map[string]any, error) {
	// 入参必须存在
	if obj == nil {
		return nil, fmt.Errorf("请求数据无效")
	}
	// obj 必须是指针，且非 nil
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil, fmt.Errorf("请求数据无效, 必须是指针类型")
	}
	// 指针必须指向结构体
	ve := v.Elem()
	if ve.Kind() != reflect.Struct {
		return nil, fmt.Errorf("请求数据无效, 必须是结构体指针类型")
	}

	// 构建白名单集合：
	// 使用 map[string]struct{} 作为“字符串集合”，只关心键是否存在，struct{} 不占用额外内存。
	// nil 表示不启用白名单（允许所有字段）；非 nil（含空切片）表示白名单集合，空集合则全部拦截。
	// 与 BuildDiffUpdatesMapJSON 共用同一套语义（均经由 makeSet），避免两种调用方对空切片理解不一致。
	allow := makeSet(allowed)

	// 输出更新数据：map[string]any 存储任意类型的值（string、int、bool 等）
	out := make(map[string]any)
	t := ve.Type()

	// 遍历结构体字段
	for i := 0; i < ve.NumField(); i++ {
		f := ve.Field(i) // 字段的值
		sf := t.Field(i) // 字段的类型定义（可读标签）

		// 仅处理“指针字段”且“非 nil”，nil 表示“未提交，不更新”
		if f.Kind() != reflect.Ptr || f.IsNil() {
			continue
		}

		// 提取标签名作为 map 键，例如 json:"company_address,omitempty" -> "company_address"
		key := sf.Tag.Get(tagName)
		if key == "" || key == "-" {
			continue
		}
		key = strings.SplitN(key, ",", 2)[0]

		// 若配置了白名单 (allow != nil)，仅允许列表中的键被加入更新集
		// 如果 allow 是空 map，表示白名单没有任何元素，所有字段都会被拦截（continue）
		if allow != nil {
			if _, ok := allow[key]; !ok {
				continue
			}
		}

		// 取指针的实际值，并按基础类型写入
		val := f.Elem()
		switch val.Kind() {
		case reflect.String:
			out[key] = val.String()
		case reflect.Bool:
			out[key] = val.Bool()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			out[key] = val.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			out[key] = val.Uint()
		case reflect.Float32, reflect.Float64:
			out[key] = val.Float()
		default:
			// 其他类型按需扩展，例如：
			// - time.Time：out[key] = val.Interface()
			// - 切片/嵌套结构体：根据业务谨慎支持
		}
	}

	// 如果没有任何可更新字段，返回错误提示
	if len(out) == 0 {
		return nil, fmt.Errorf("没有要更新的字段")
	}
	return out, nil
}

// BuildUpdatesMapJSON 是基于 json 标签的便捷封装
// 功能描述:
//
//	等价于调用 BuildUpdatesMap(obj, "json", nil)
//
// 参数说明:
//   - obj: any，必须是结构体指针。
//
// 返回值:
//   - map[string]any: 更新集合，仅包含“非 nil 指针字段”。
//   - error: 参数非法或没有任何可更新字段时返回错误。
//
// 调用示例:
//
//	updates, err := BuildUpdatesMapJSON(&req)
func BuildUpdatesMapJSON(obj any) (map[string]any, error) {
	return BuildUpdatesMap(obj, "json", nil)
}

// BuildDiffUpdatesMapJSON 比较新旧对象（按 json 标签对应字段），仅生成“发生变化”的更新集
// 功能描述:
//
//	对比 newObj 与 oldObj 中携带的字段（按 json 标签匹配），仅当“新值与旧值不同”才加入更新集；指针字段为 nil 表示“不更新”。
//	列名优先使用 oldObj 中的 gorm 标签 column:xxx，缺省回退为 json 键。
//
// 参数说明:
//   - newObj: any，请求新值对象，必须为结构体指针；字段可为指针或非指针。
//   - oldObj: any，旧值对象，必须为结构体指针；用于读取旧值与 gorm 列名。
//   - allowed: []string，可选白名单（按 json 键）；nil 表示不过滤，允许所有匹配 json 键的字段参与比较。
//
// 返回值:
//   - map[string]any: 仅包含“发生变化”的更新项，键为列名（优先 gorm column）。
//   - error: 参数非法或没有任何可更新字段时返回错误。
//
// 调用示例:
//
//	updates, err := BuildDiffUpdatesMapJSON(&req, &old, []string{"status","send_log"})
//	updates, err := BuildDiffUpdatesMapJSON(&req, &old, nil) // 不启用白名单
func BuildDiffUpdatesMapJSON(newObj any, oldObj any, allowed []string) (map[string]any, error) {
	return buildDiffUpdatesMap(newObj, oldObj, "json", "gorm", allowed)
}

// buildDiffUpdatesMap 执行差异提取与列名映射（内部函数）
// 功能描述:
//
//	读取 newObj 与 oldObj 的字段（支持匿名嵌入），按 tagName 对齐匹配；指针字段为 nil 表示不更新；列名优先解析 gormTagName 中的 column:xxx。
//
// 参数说明:
//   - newObj: any，请求对象（结构体指针）
//   - oldObj: any，旧值对象（结构体指针）
//   - tagName: string，键标签（通常为 "json"）
//   - gormTagName: string，列名标签（固定为 "gorm"）
//   - allowed: []string，白名单（按键名），nil 表示不过滤
//
// 返回值:
//   - map[string]any: 发生变化的列更新集合
//   - error: 参数非法或无更新项
func buildDiffUpdatesMap(newObj any, oldObj any, tagName, gormTagName string, allowed []string) (map[string]any, error) {
	if newObj == nil || oldObj == nil {
		return nil, fmt.Errorf("请求数据无效")
	}
	nv := reflect.ValueOf(newObj)
	ov := reflect.ValueOf(oldObj)
	if nv.Kind() != reflect.Ptr || nv.IsNil() || ov.Kind() != reflect.Ptr || ov.IsNil() {
		return nil, fmt.Errorf("请求数据无效, 必须是指针类型")
	}
	ne := nv.Elem()
	oe := ov.Elem()
	if ne.Kind() != reflect.Struct || oe.Kind() != reflect.Struct {
		return nil, fmt.Errorf("请求数据无效, 必须是结构体指针类型")
	}

	// 构建白名单集合：nil 表示不启用白名单过滤
	allow := makeSet(allowed)

	// 旧值：json 标签 -> 旧值 + 列名
	oldVals := make(map[string]reflect.Value)
	columns := make(map[string]string)
	collectOldFields(oe, oe.Type(), tagName, gormTagName, oldVals, columns)

	// 新值：遍历并比较，构造更新集
	out := make(map[string]any)
	collectNewAndCompareFields(ne, ne.Type(), tagName, allow, oldVals, columns, out)

	if len(out) == 0 {
		return nil, fmt.Errorf("没有要更新的字段")
	}
	return out, nil
}

// makeSet 将字符串切片转为集合（map[string]struct{}）
// 功能描述:
//
//	便于 O(1) 判断指定键是否在白名单中；当 list 为 nil 时返回 nil，表示不启用白名单。
//	注意：如果 list 是空切片 []string{}，会返回一个空的 map，表示“没有任何字段在白名单中（全部拦截）”。
func makeSet(list []string) map[string]struct{} {
	if list == nil {
		return nil
	}
	m := make(map[string]struct{}, len(list))
	for _, s := range list {
		if s != "" {
			m[s] = struct{}{}
		}
	}
	return m
}

// firstTagKey 提取标签的首个键名（去除 ",omitempty" 等修饰）
// 参数:
//   - tag: string，形如 "company_address,omitempty"
//
// 返回:
//   - string: 提取的键名；为空或为 "-" 时返回空字符串
func firstTagKey(tag string) string {
	key := strings.SplitN(tag, ",", 2)[0]
	if key == "" || key == "-" {
		return ""
	}
	return key
}

// parseGormColumnTag 解析 gorm 标签中的列名定义
// 功能描述:
//
//	从形如 "column:uid;type:bigint;index" 的标签中抽取列名（uid）。
func parseGormColumnTag(tag string) string {
	if tag == "" {
		return ""
	}
	parts := strings.SplitSeq(tag, ";")
	for p := range parts {
		p = strings.TrimSpace(p)
		if after, ok := strings.CutPrefix(p, "column:"); ok {
			return after
		}
	}
	return ""
}

// collectOldFields 收集旧值对象的“键 -> 值”和“键 -> 列名”映射
// 功能描述:
//
//	遍历 oldObj（支持匿名嵌入），记录 json 键对应的旧值与列名。
func collectOldFields(v reflect.Value, t reflect.Type, tagName, gormTagName string, values map[string]reflect.Value, columns map[string]string) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		fv := v.Field(i)

		// 递归处理匿名嵌入字段（如 data.Declare 内嵌 biz.Declare）
		if sf.Anonymous && fv.Kind() == reflect.Struct {
			collectOldFields(fv, fv.Type(), tagName, gormTagName, values, columns)
			continue
		}

		key := firstTagKey(sf.Tag.Get(tagName))
		if key == "" {
			continue
		}
		values[key] = fv

		col := parseGormColumnTag(sf.Tag.Get(gormTagName))
		if col == "" {
			col = key // 回退为 json 键（要求与列名一致，否则需要白名单或映射）
		}
		columns[key] = col
	}
}

// collectNewAndCompareFields 遍历新值对象并与旧值比较，生成更新集
// 功能描述:
//
//	指针字段为 nil 表示“不更新”；非指针字段始终参与比较。当旧值不存在或新旧值不相等时，加入更新集（键为列名）。
func collectNewAndCompareFields(v reflect.Value, t reflect.Type, tagName string, allow map[string]struct{}, oldVals map[string]reflect.Value, columns map[string]string, out map[string]any) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		fv := v.Field(i)

		// 递归处理匿名嵌入字段
		if sf.Anonymous && fv.Kind() == reflect.Struct {
			collectNewAndCompareFields(fv, fv.Type(), tagName, allow, oldVals, columns, out)
			continue
		}

		key := firstTagKey(sf.Tag.Get(tagName))
		if key == "" {
			continue
		}
		if allow != nil {
			if _, ok := allow[key]; !ok {
				continue
			}
		}

		// 指针且为 nil 表示“不更新”
		if fv.Kind() == reflect.Ptr && fv.IsNil() {
			continue
		}

		newVal := fv
		if newVal.Kind() == reflect.Ptr {
			newVal = newVal.Elem()
		}

		oldVal, ok := oldVals[key]
		if !ok || !valuesEqual(newVal, oldVal) {
			col := columns[key]
			if col == "" {
				col = key
			}
			out[col] = valueAsInterface(newVal)
		}
	}
}

// valuesEqual 比较两个值是否相等（支持指针解引用与基础数值类型）
func valuesEqual(a reflect.Value, b reflect.Value) bool {
	if !b.IsValid() {
		return false
	}
	ak := a.Kind()
	bk := b.Kind()
	if ak == reflect.Ptr {
		if a.IsNil() {
			return b.IsNil()
		}
		a = a.Elem()
		ak = a.Kind()
	}
	if bk == reflect.Ptr {
		if b.IsNil() {
			return a.IsNil()
		}
		b = b.Elem()
		bk = b.Kind()
	}

	switch {
	case ak == reflect.String && bk == reflect.String:
		return a.String() == b.String()
	case ak == reflect.Bool && bk == reflect.Bool:
		return a.Bool() == b.Bool()
	case isIntKind(ak) && isIntKind(bk):
		return a.Int() == b.Int()
	case isUintKind(ak) && isUintKind(bk):
		return a.Uint() == b.Uint()
	case isFloatKind(ak) && isFloatKind(bk):
		return a.Float() == b.Float()
	default:
		return reflect.DeepEqual(a.Interface(), b.Interface())
	}
}

// isIntKind 判断是否为整数类型（含各位宽）
func isIntKind(k reflect.Kind) bool {
	return k == reflect.Int || k == reflect.Int8 || k == reflect.Int16 || k == reflect.Int32 || k == reflect.Int64
}

// isUintKind 判断是否为无符号整数类型（含各位宽）
func isUintKind(k reflect.Kind) bool {
	return k == reflect.Uint || k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64
}

// isFloatKind 判断是否为浮点数类型
func isFloatKind(k reflect.Kind) bool {
	return k == reflect.Float32 || k == reflect.Float64
}

// valueAsInterface 将 reflect.Value 转为具体基础类型的 interface{}
// 功能描述:
//
//	自动解引用指针，将基础类型统一转换为其 Go 对应类型（string/bool/int/uint/float），其他类型返回原始 Interface。
func valueAsInterface(v reflect.Value) any {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	default:
		return v.Interface()
	}
}
