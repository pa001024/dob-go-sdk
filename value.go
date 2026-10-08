// JSON 值模型与取值助手：原始表/设置/BD 全是 map[string]any，数值一律 float64。
package dob

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// V 是解码后的 JSON 值（float64 / string / bool / nil / []any / map[string]any）。
type V = any

// Num 宽松转数值：bool 按 Python 口径（evaluate 侧 true→1，Bonus 侧见 NumOrZero）。
func Num(v V) float64 {
	switch t := v.(type) {
	case bool:
		if t {
			return 1
		}
		return 0
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0
		}
		return f
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// NumOrZero 加成表口径：bool 一律 0，其余与 Num 一致（含 json.Number）。
func NumOrZero(v V) float64 {
	if _, ok := v.(bool); ok {
		return 0
	}
	return Num(v)
}

// IDKey 将 id 值规范化为字符串键（int/float64/json.Number 同值同键）。
func IDKey(v V) string {
	if IsNum(v) {
		return strconv.FormatFloat(Num(v), 'f', -1, 64)
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// IsNum 数值判定（bool 除外，对齐 entities._is_num）。
func IsNum(v V) bool {
	switch v.(type) {
	case float64, float32, int, int64, json.Number:
		return true
	default:
		return false
	}
}

// IsFiniteNum 数值且有限。
func IsFiniteNum(v V) bool {
	if !IsNum(v) {
		return false
	}
	f := Num(v)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// S 取字符串字段。
func S(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// F 取数值字段（缺省 0）。
func F(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	return NumOrZero(m[key])
}

// L 取数组字段。
func L(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	if l, ok := m[key].([]any); ok {
		return l
	}
	return nil
}

// M 取对象字段。
func M(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if mm, ok := m[key].(map[string]any); ok {
		return mm
	}
	return nil
}

// AsMap 安全断言 map。
func AsMap(v V) map[string]any {
	if mm, ok := v.(map[string]any); ok {
		return mm
	}
	return nil
}

// AsList 安全断言 list（兼容 []any 与其他切片类型如 []map[string]any）。
func AsList(v V) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out = append(out, rv.Index(i).Interface())
		}
		return out
	}
	return nil
}

// CloneMap 浅拷贝 map。
func CloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// KeyOf 渲染 attackBonusLog 键（含 None 口径：nil → "None"，与 Python f-string 一致）。
func KeyOf(parts ...V) string {
	strs := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == nil {
			strs = append(strs, "None")
			continue
		}
		strs = append(strs, fmt.Sprint(p))
	}
	return strings.Join(strs, "::")
}

// DecodeJSON 用 UseNumber 解码（整数保持 json.Number，避免 float 精度污染 id）。
func DecodeJSON(data []byte) (V, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var v V
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// ToFloatSecond 精度：id 等大整数比较用（json.Number 已保留原文）。
func IntOf(v V) int {
	return int(math.Floor(Num(v) + 0.5))
}
