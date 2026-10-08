// 按模块按需加载：经 gameData GraphQL 分页拉取数据集，磁盘 + 内存两级持久化。
package dob

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxPage 单页拉取上限。
const MaxPage = 500

func safeDatasetName(dataset string) string {
	r := strings.ReplaceAll(dataset, ":", "__")
	return strings.ReplaceAll(r, "/", "__")
}

func defaultModuleCacheDir() string {
	if override := os.Getenv("DNA_BUILDER_CACHE"); override != "" {
		return filepath.Join(override, "gamedata")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".cache", "dna-builder", "gamedata")
}

type modulePayload struct {
	Dataset   string  `json:"dataset"`
	Kind      string  `json:"kind"`
	Total     int     `json:"total"`
	Items     []any   `json:"items"`
	Value     any     `json:"value"`
	UpdatedAt float64 `json:"updatedAt"`
}

// GameDataQuerier 是 ModuleStore 所需的拉取面（GameDataClient 实现；测试可用 fake）。
type GameDataQuerier interface {
	IterAll(dataset string, q GameDataQuery, yield func(page map[string]any) bool) error
	Record(dataset, key string) (map[string]any, error)
}

// ModuleStore 是 dataset 粒度缓存：内存热缓存 + 磁盘 JSON，Fetch 即热重载。
//
// 同时实现原始表数据源抽象接口（LoadTables），可直接给 Engine 使用。
// 模块列表在声明时确定（modules 表名子集 / mapping 表→数据集全量映射，
// 缺省 8 张全量表），LoadTables 只拉取声明过的表。
type ModuleStore struct {
	GameData GameDataQuerier
	CacheDir string
	Mapping  map[string]string
	memory   map[string]*modulePayload
	mtimes   map[string]float64
}

// NewModuleStore 构造（cacheDir 为空用默认；modules 为表名子集；mapping 优先）。
func NewModuleStore(gameData GameDataQuerier, cacheDir string, modules []string, mapping map[string]string) (*ModuleStore, error) {
	if cacheDir == "" {
		cacheDir = defaultModuleCacheDir()
	}
	_ = os.MkdirAll(cacheDir, 0o755)
	m := map[string]string{}
	if mapping != nil {
		for k, v := range mapping {
			m[k] = v
		}
	} else if modules != nil {
		var unknown []string
		for _, mod := range modules {
			if _, ok := DatasetDefaults[mod]; !ok {
				unknown = append(unknown, mod)
			}
		}
		if len(unknown) > 0 {
			return nil, &DobApiError{Message: "未知原始表", Code: "unknown_table", Payload: unknown}
		}
		for _, mod := range modules {
			m[mod] = DatasetDefaults[mod]
		}
	} else {
		for k, v := range DatasetDefaults {
			m[k] = v
		}
	}
	return &ModuleStore{GameData: gameData, CacheDir: cacheDir, Mapping: m, memory: map[string]*modulePayload{}, mtimes: map[string]float64{}}, nil
}

// PathFor 数据集对应的磁盘路径。
func (s *ModuleStore) PathFor(dataset string) string {
	return filepath.Join(s.CacheDir, safeDatasetName(dataset)+".json")
}

// Get 取 dataset 全量 items：内存 → 磁盘 →（可选）远端分页拉取。
func (s *ModuleStore) Get(dataset string, autoFetch bool, q GameDataQuery) ([]any, error) {
	if p, ok := s.memory[dataset]; ok {
		return p.Items, nil
	}
	if p := s.readDisk(dataset); p != nil {
		s.memory[dataset] = p
		return p.Items, nil
	}
	if !autoFetch {
		return []any{}, nil
	}
	return s.Fetch(dataset, "array", q)
}

// Record 按键取单条：先查本地缓存，缺失则走 gameDataRecord（不拉全量）。
func (s *ModuleStore) Record(dataset, key string, q GameDataQuery) (map[string]any, error) {
	items, err := s.Get(dataset, false, q)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			var k string
			k, _ = m["key"].(string)
			if k == key {
				return m, nil
			}
		}
	}
	if _, ok := s.memory[dataset]; !ok && s.readDisk(dataset) == nil {
		rec, err := s.GameData.Record(dataset, key)
		if err != nil {
			return nil, err
		}
		if rec == nil {
			return nil, nil
		}
		return map[string]any{"key": key, "data": rec["data"]}, nil
	}
	return nil, nil
}

// Fetch 远端分页拉全量并持久化，内存热替换（运行时热重载入口）。
func (s *ModuleStore) Fetch(dataset, kind string, q GameDataQuery) ([]any, error) {
	var items []any
	total := 0
	err := s.GameData.IterAll(dataset, q, func(page map[string]any) bool {
		total = int(Num(page["total"]))
		items = append(items, AsList(page["items"])...)
		return true
	})
	if err != nil {
		return nil, err
	}
	value := NormalizeItems(items, kind)
	if total == 0 {
		total = len(items)
	}
	p := &modulePayload{Dataset: dataset, Kind: kind, Total: total, Items: items, Value: value, UpdatedAt: float64(time.Now().UnixNano()) / 1e9}
	if err := s.writeDisk(dataset, p); err != nil {
		return nil, err
	}
	s.memory[dataset] = p
	return items, nil
}

// GetValue 取 dataset 归一化原始值（与 datapack 模块值同形）。
func (s *ModuleStore) GetValue(dataset, kind string, autoFetch bool, q GameDataQuery) (any, error) {
	if p, ok := s.memory[dataset]; ok {
		if p.Value != nil && (p.Kind == kind || p.Kind == "") {
			return p.Value, nil
		}
		value := NormalizeItems(p.Items, kind)
		p.Value = value
		p.Kind = kind
		return value, nil
	}
	if p := s.readDisk(dataset); p != nil {
		s.memory[dataset] = p
		return s.GetValue(dataset, kind, false, q)
	}
	if !autoFetch {
		if kind == "array" {
			return []any{}, nil
		}
		return map[string]any{}, nil
	}
	if _, err := s.Fetch(dataset, kind, q); err != nil {
		return nil, err
	}
	return s.memory[dataset].Value, nil
}

// Reload 强制重新拉取（丢弃内存与磁盘旧值）。
func (s *ModuleStore) Reload(dataset string, q GameDataQuery) ([]any, error) {
	return s.Fetch(dataset, "array", q)
}

// Datasets 声明过的 dataset id 列表（去重保序）。
func (s *ModuleStore) Datasets() []string {
	var out []string
	seen := map[string]bool{}
	for _, table := range s.Tables() {
		ds := s.Mapping[table]
		if !seen[ds] {
			seen[ds] = true
			out = append(out, ds)
		}
	}
	return out
}

// Tables 声明过的原始表名列表。
func (s *ModuleStore) Tables() []string {
	var out []string
	for _, table := range []string{"chars", "mods", "buffs", "effects", "weapons", "pets", "pet_entries", "monsters"} {
		if _, ok := s.Mapping[table]; ok {
			out = append(out, table)
		}
	}
	// 声明顺序外的新增表（mapping 自定义时）
	for table := range s.Mapping {
		found := false
		for _, t := range out {
			if t == table {
				found = true
				break
			}
		}
		if !found {
			out = append(out, table)
		}
	}
	return out
}

// Preload 预拉取声明过的所有模块（去重按 dataset）。
func (s *ModuleStore) Preload(q GameDataQuery) (map[string]int, error) {
	out := map[string]int{}
	for _, ds := range s.Datasets() {
		items, err := s.Fetch(ds, "array", q)
		if err != nil {
			return nil, err
		}
		out[ds] = len(items)
	}
	return out, nil
}

// LoadTables 数据源抽象接口实现：只拉取声明时的模块列表。
func (s *ModuleStore) LoadTables() (*GameDataTables, error) {
	raw := map[string]any{}
	for _, table := range s.Tables() {
		v, err := s.GetValue(s.Mapping[table], "array", true, GameDataQuery{})
		if err != nil {
			return nil, err
		}
		raw[table] = v
	}
	return NewGameDataTables(raw), nil
}

// ReloadAll 批量重载。
func (s *ModuleStore) ReloadAll(datasets []string, q GameDataQuery) (map[string]int, error) {
	if datasets == nil {
		datasets = s.CachedDatasets()
	}
	out := map[string]int{}
	for _, ds := range datasets {
		items, err := s.Fetch(ds, "array", q)
		if err != nil {
			return nil, err
		}
		out[ds] = len(items)
	}
	return out, nil
}

// RefreshIfChanged 磁盘文件被外部改写时重载内存。
func (s *ModuleStore) RefreshIfChanged(dataset string) bool {
	fi, err := os.Stat(s.PathFor(dataset))
	if err != nil {
		return false
	}
	mtime := float64(fi.ModTime().UnixNano()) / 1e9
	if s.mtimes[dataset] != mtime {
		if p := s.readDisk(dataset); p != nil {
			s.memory[dataset] = p
			return true
		}
	}
	return false
}

// Invalidate 丢弃内存与磁盘缓存。
func (s *ModuleStore) Invalidate(dataset string) {
	delete(s.memory, dataset)
	_ = os.Remove(s.PathFor(dataset))
	delete(s.mtimes, dataset)
}

// CachedDatasets 已缓存的数据集（内存 + 磁盘）。
func (s *ModuleStore) CachedDatasets() []string {
	set := map[string]bool{}
	for k := range s.memory {
		set[k] = true
	}
	if entries, err := os.ReadDir(s.CacheDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasSuffix(name, ".json") {
				set[strings.ReplaceAll(name[:len(name)-5], "__", ":")] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (s *ModuleStore) readDisk(dataset string) *modulePayload {
	path := s.PathFor(dataset)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var p modulePayload
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&p); err != nil {
		return nil
	}
	if fi, err := os.Stat(path); err == nil {
		s.mtimes[dataset] = float64(fi.ModTime().UnixNano()) / 1e9
	}
	return &p
}

func (s *ModuleStore) writeDisk(dataset string, p *modulePayload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tmp := s.PathFor(dataset) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.PathFor(dataset)); err != nil {
		return err
	}
	if fi, err := os.Stat(s.PathFor(dataset)); err == nil {
		s.mtimes[dataset] = float64(fi.ModTime().UnixNano()) / 1e9
	}
	return nil
}
