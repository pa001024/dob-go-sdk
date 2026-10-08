// 测试助手：fixture 加载与断言。
package dob

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func goldenDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "python", "tests", "golden")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("golden 目录缺失: %v", err)
	}
	return dir
}

func loadGolden(t *testing.T, name string) any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenDir(t), name))
	if err != nil {
		t.Fatalf("读 %s 失败: %v", name, err)
	}
	v, err := DecodeJSON(raw)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", name, err)
	}
	return v
}

func tablesFor(t *testing.T, data map[string]any) *GameDataTables {
	t.Helper()
	// fixture 键名 petEntries → 表名 pet_entries（与 Python 测试同形）。
	keyOf := map[string]string{
		"chars": "chars", "mods": "mods", "buffs": "buffs", "effects": "effects",
		"weapons": "weapons", "pets": "pets", "pet_entries": "petEntries", "monsters": "monsters",
	}
	raw := map[string]any{}
	for _, table := range []string{"chars", "mods", "buffs", "effects", "weapons", "pets", "pet_entries", "monsters"} {
		var list []any
		for _, e := range AsList(data[keyOf[table]]) {
			if e == nil {
				t.Fatalf("原始表 %s 含 null 条目", table)
			}
			list = append(list, e)
		}
		raw[table] = list
	}
	tables, err := TablesFromDict(raw)
	if err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return tables
}

func isClose(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	diff := math.Abs(a - b)
	if diff <= 1e-12 {
		return true
	}
	rel := diff / math.Max(math.Abs(a), math.Abs(b))
	return rel <= 1e-9
}

func assertClose(t *testing.T, what string, got, want float64) {
	t.Helper()
	if !isClose(got, want) {
		t.Errorf("%s: mine=%v ref=%v", what, got, want)
	}
}
