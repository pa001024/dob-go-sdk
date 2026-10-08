// 数据源抽象接口用例（纯本地，无网络）。
package dob

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeTableGameData struct {
	values map[string][]any
}

func newFakeTableGameData(t *testing.T) *fakeTableGameData {
	t.Helper()
	data := loadGolden(t, "shardBuild1_data.json").(map[string]any)
	var effects []any
	for _, e := range AsList(data["effects"]) {
		if e != nil {
			effects = append(effects, e)
		}
	}
	var entries []any
	for _, e := range AsList(data["petEntries"]) {
		entries = append(entries, e)
	}
	return &fakeTableGameData{values: map[string][]any{
		"char": data["chars"].([]any), "mod": data["mods"].([]any),
		"buff": data["buffs"].([]any), "effect": effects,
		"weapon": data["weapons"].([]any), "pet": data["pets"].([]any),
		"pet:petEntrys": entries, "monster": data["monsters"].([]any),
	}}
}

func (f *fakeTableGameData) IterAll(dataset string, q GameDataQuery, yield func(page map[string]any) bool) error {
	values := f.values[dataset]
	var items []any
	for i, v := range values {
		items = append(items, map[string]any{"key": itoa(i), "data": v})
	}
	yield(map[string]any{"total": len(values), "items": items})
	return nil
}

func (f *fakeTableGameData) Record(dataset, key string) (map[string]any, error) { return nil, nil }

func TestDefaultIsFullMapping(t *testing.T) {
	dir, err := os.MkdirTemp("", "dob-ts")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store, err := NewModuleStore(newFakeTableGameData(t), dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.Mapping, DatasetDefaults) {
		t.Errorf("mapping: %v", store.Mapping)
	}
}

func TestModulesSubset(t *testing.T) {
	store, err := NewModuleStore(newFakeTableGameData(t), filepath.Join(os.TempDir(), "dob-x"), []string{"chars", "mods"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.Mapping, map[string]string{"chars": "char", "mods": "mod"}) {
		t.Errorf("mapping: %v", store.Mapping)
	}
	datasets := store.Datasets()
	if len(datasets) != 2 || datasets[0] != "char" || datasets[1] != "mod" {
		t.Errorf("datasets: %v", datasets)
	}
}

func TestUnknownModuleRejected(t *testing.T) {
	if _, err := NewModuleStore(newFakeTableGameData(t), "", []string{"nope"}, nil); err == nil {
		t.Errorf("未知表应报错")
	}
}

func TestCustomMapping(t *testing.T) {
	store, err := NewModuleStore(newFakeTableGameData(t), "", nil, map[string]string{"chars": "char"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.Mapping, map[string]string{"chars": "char"}) {
		t.Errorf("mapping: %v", store.Mapping)
	}
}

func TestModuleStoreLoadTablesForEngine(t *testing.T) {
	dir, err := os.MkdirTemp("", "dob-ts")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store, err := NewModuleStore(newFakeTableGameData(t), dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tables, err := store.LoadTables()
	if err != nil {
		t.Fatal(err)
	}
	if len(tables.Chars) == 0 || len(tables.Mods) == 0 {
		t.Errorf("表为空")
	}
	bd := loadGolden(t, "shardBuild1_bd.json").(map[string]any)
	settings, _ := bd["settings"].(map[string]any)
	state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v := NewEngine(state, tables).Calculate(""); !IsFiniteNum(v) {
		t.Errorf("求值非有限: %v", v)
	}
}

func TestModuleStoreSubsetLoadTables(t *testing.T) {
	dir, err := os.MkdirTemp("", "dob-ts")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store, err := NewModuleStore(newFakeTableGameData(t), dir, []string{"chars", "mods"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tables, err := store.LoadTables()
	if err != nil {
		t.Fatal(err)
	}
	if len(tables.Chars) == 0 || len(tables.Mods) == 0 || len(tables.Weapons) != 0 {
		t.Errorf("子集错误")
	}
}

func TestDataPackLoadTables(t *testing.T) {
	dir, err := os.MkdirTemp("", "dob-ts")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	pack := NewDataPackStore(dir, "", "-")
	pack.activeVersion = "stub"
	pack.moduleCache = map[string]map[string]any{
		"char.data":    {"default": []any{map[string]any{"id": 1, "名称": "a"}}},
		"mod.data":     {"default": []any{}},
		"buff.data":    {"default": []any{}},
		"effect.data":  {"default": []any{}},
		"weapon.data":  {"default": []any{}},
		"pet.data":     {"default": []any{}, "petEntrys": []any{}},
		"monster.data": {"default": []any{}},
	}
	tables, err := pack.LoadTables()
	if err != nil {
		t.Fatal(err)
	}
	if len(tables.Chars) != 1 {
		t.Errorf("chars: %d", len(tables.Chars))
	}
}

type stubSource struct{ t *GameDataTables }

func (s stubSource) LoadTables() (*GameDataTables, error) { return s.t, nil }

func TestResolveTables(t *testing.T) {
	data := loadGolden(t, "shardBuild1_data.json").(map[string]any)
	tables := tablesFor(t, data)
	if got, err := ResolveTables(tables); err != nil || got != tables {
		t.Errorf("原样返回失败: %v %v", got, err)
	}
	if got, err := ResolveTables(stubSource{tables}); err != nil || got != tables {
		t.Errorf("source 失败: %v %v", got, err)
	}
	if _, err := ResolveTables(42); err == nil {
		t.Errorf("非法源应报错")
	}
}

func TestSnapshotFromSource(t *testing.T) {
	data := loadGolden(t, "shardBuild1_data.json").(map[string]any)
	tables := tablesFor(t, data)
	bd := loadGolden(t, "shardBuild1_bd.json").(map[string]any)
	expr := loadGolden(t, "shardBuild1_expr.json").(map[string]any)
	srv := fakeBuildServer(demoBuilds(t, bd))
	defer srv.Close()
	snapshot, err := SnapshotFromOnline("demo", SnapshotOptions{
		Backend: NewBackendClient(srv.URL, 0), Source: stubSource{tables},
	})
	if err != nil {
		t.Fatal(err)
	}
	var target float64
	for _, c := range expr["cases"].([]any) {
		if cm, ok := c.(map[string]any); ok && cm["expr"] == S(AsMap(bd["settings"]), "targetFunction") {
			target = Num(cm["ts"])
		}
	}
	assertClose(t, "target", snapshot.Calculate(""), target)
}
