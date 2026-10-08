// gameData ↔ datapack 形状归一化用例（纯本地，无网络）。
package dob

import (
	"os"
	"testing"
)

type fakeGameData struct {
	pages []map[string]any
	calls int
}

func (f *fakeGameData) IterAll(dataset string, q GameDataQuery, yield func(page map[string]any) bool) error {
	f.calls++
	for _, p := range f.pages {
		items := AsList(p["items"])
		total := 0
		for _, q := range f.pages {
			total += len(AsList(q["items"]))
		}
		if !yield(map[string]any{"total": total, "items": items}) {
			break
		}
	}
	return nil
}

func (f *fakeGameData) Record(dataset, key string) (map[string]any, error) { return nil, nil }

func TestNormalizeArray(t *testing.T) {
	items := []any{
		map[string]any{"key": "1101", "data": map[string]any{"id": 1101, "名称": "a"}},
		map[string]any{"key": "1102", "data": map[string]any{"id": 1102}},
	}
	list, ok := NormalizeItems(items, "array").([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("got %v", NormalizeItems(items, "array"))
	}
	if Num(AsMap(list[0])["id"]) != 1101 {
		t.Errorf("got %v", list[0])
	}
}

func TestNormalizeObject(t *testing.T) {
	items := []any{
		map[string]any{"key": "a", "data": map[string]any{"x": 1}},
		map[string]any{"key": "b", "data": map[string]any{"x": 2}},
	}
	got, ok := NormalizeItems(items, "object").(map[string]any)
	if !ok || Num(AsMap(got["a"])["x"]) != 1 || Num(AsMap(got["b"])["x"]) != 2 {
		t.Errorf("got %v", got)
	}
}

func TestValueWrapper(t *testing.T) {
	items := []any{
		map[string]any{"key": "0", "data": map[string]any{"value": 5}},
		map[string]any{"key": "1", "data": map[string]any{"value": "s"}},
	}
	got, _ := NormalizeItems(items, "array").([]any)
	if len(got) != 2 || Num(got[0]) != 5 || got[1].(string) != "s" {
		t.Errorf("got %v", got)
	}
}

func TestStoreValueRoundtrip(t *testing.T) {
	dir, err := os.MkdirTemp("", "dob-gm")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	fake := &fakeGameData{pages: []map[string]any{
		{"total": 2, "items": []any{
			map[string]any{"key": "1", "data": map[string]any{"id": 1}},
			map[string]any{"key": "2", "data": map[string]any{"id": 2}},
		}},
	}}
	store, err := NewModuleStore(fake, dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := store.GetValue("mod", "array", true, GameDataQuery{})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := v.([]any)
	if len(list) != 2 || Num(AsMap(list[0])["id"]) != 1 {
		t.Fatalf("got %v", v)
	}
	// 新实例只读磁盘，不再请求远端
	fake2 := &fakeGameData{}
	store2, err := NewModuleStore(fake2, dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := store2.GetValue("mod", "array", false, GameDataQuery{})
	if err != nil {
		t.Fatal(err)
	}
	list2, _ := v2.([]any)
	if len(list2) != 2 {
		t.Fatalf("got %v", v2)
	}
	if fake2.calls != 0 {
		t.Errorf("不应请求远端")
	}
}

func TestStoreObjectKind(t *testing.T) {
	dir, err := os.MkdirTemp("", "dob-gm")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	fake := &fakeGameData{pages: []map[string]any{
		{"total": 1, "items": []any{map[string]any{"key": "k", "data": map[string]any{"v": 1}}}},
	}}
	store, err := NewModuleStore(fake, dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := store.GetValue("mod", "object", true, GameDataQuery{})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := v.(map[string]any)
	if !ok || Num(AsMap(m["k"])["v"]) != 1 {
		t.Errorf("got %v", v)
	}
}
