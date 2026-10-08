// gameData* GraphQL 封装：口径与服务端 game-data-api.md 一致，limit 上限 500。
package dob

// MaxLimit 单页上限，DefaultLimit 默认分页。
const (
	MaxLimit     = 500
	DefaultLimit = 50
)

// UnwrapValue 解开原始值导出的 {value} 包装（对齐 gameDataRegistry 记录标准化）。
func UnwrapValue(data V) V {
	if m, ok := data.(map[string]any); ok && len(m) == 1 {
		if v, ok := m["value"]; ok {
			return v
		}
	}
	return data
}

// NormalizeItems 将 gameData items[{key, data}] 归一化为 datapack 同形的原始值。
func NormalizeItems(items []any, kind string) any {
	if kind == "object" || kind == "map" {
		out := make(map[string]any, len(items))
		for _, it := range items {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			key, _ := m["key"].(string)
			out[key] = UnwrapValue(m["data"])
		}
		return out
	}
	out := make([]any, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, UnwrapValue(m["data"]))
	}
	return out
}

// GameDataClient 封装 gameData* 查询。
type GameDataClient struct {
	Backend *BackendClient
}

// NewGameDataClient 构造。
func NewGameDataClient(backend *BackendClient) *GameDataClient {
	return &GameDataClient{Backend: backend}
}

// Modules 数据模块列表。
func (g *GameDataClient) Modules() ([]any, error) {
	data, err := g.Backend.GraphQL("{gameDataModules{id label file baseId locale}}", nil)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "gameDataModules"), nil
}

// Sets 数据集列表。
func (g *GameDataClient) Sets(module *string) ([]any, error) {
	data, err := g.Backend.GraphQL(
		"query($m:String){gameDataSets(module:$m){id exportName kind count locale}}",
		map[string]any{"m": optStr(module)},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "gameDataSets"), nil
}

// Fields 数据集字段列表。
func (g *GameDataClient) Fields(dataset string, limit int) ([]any, error) {
	data, err := g.Backend.GraphQL(
		"query($d:String!,$l:Int){gameDataFields(dataset:$d,limit:$l)}",
		map[string]any{"d": dataset, "l": limit},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "gameDataFields"), nil
}

// GameDataQuery 单页查询参数。
type GameDataQuery struct {
	Where  []any
	Search *string
	Sort   []any
	Fields []string
	Offset int
	Limit  int
}

// Query 单页查询，limit 自动 clamp 到 [1, 500]。
func (g *GameDataClient) Query(dataset string, q GameDataQuery) (map[string]any, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	var search any
	if q.Search != nil {
		search = *q.Search
	}
	var fields any
	if q.Fields != nil {
		fields = q.Fields
	}
	where := q.Where
	if where == nil {
		where = []any{}
	}
	sort := q.Sort
	if sort == nil {
		sort = []any{}
	}
	data, err := g.Backend.GraphQL(
		"query($i:GameDataQuery!){gameData(input:$i){dataset total count offset limit items{key data}}}",
		map[string]any{"i": map[string]any{
			"dataset": dataset,
			"where":   where,
			"search":  search,
			"sort":    sort,
			"fields":  fields,
			"offset":  q.Offset,
			"limit":   limit,
		}},
	)
	if err != nil {
		return nil, err
	}
	if gd, ok := data["gameData"].(map[string]any); ok {
		return gd, nil
	}
	return map[string]any{"dataset": dataset, "total": 0, "items": []any{}}, nil
}

// Record 按键取单条。
func (g *GameDataClient) Record(dataset, key string) (map[string]any, error) {
	data, err := g.Backend.GraphQL(
		"query($d:String!,$k:String!){gameDataRecord(dataset:$d,key:$k){key data}}",
		map[string]any{"d": dataset, "k": key},
	)
	if err != nil {
		return nil, err
	}
	return gqlMap(data, "gameDataRecord"), nil
}

// FieldValues 字段值分布。
func (g *GameDataClient) FieldValues(dataset, field string, limit int) ([]any, error) {
	data, err := g.Backend.GraphQL(
		"query($d:String!,$f:String!,$l:Int){gameDataFieldValues(dataset:$d,field:$f,limit:$l){value count}}",
		map[string]any{"d": dataset, "f": field, "l": limit},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "gameDataFieldValues"), nil
}

// IterAll 按分页拉全量，对每页调用 yield；total 以首页为准。
func (g *GameDataClient) IterAll(dataset string, q GameDataQuery, yield func(page map[string]any) bool) error {
	offset := 0
	for {
		qq := q
		qq.Offset = offset
		qq.Limit = MaxLimit
		page, err := g.Query(dataset, qq)
		if err != nil {
			return err
		}
		if !yield(page) {
			return nil
		}
		items := AsList(page["items"])
		total := int(Num(page["total"]))
		offset += len(items)
		if len(items) == 0 || offset >= total {
			break
		}
	}
	return nil
}

// FetchAll 拉全量 items（大数据集慎用，优先走 ModuleStore 持久化）。
func (g *GameDataClient) FetchAll(dataset string, q GameDataQuery) ([]any, error) {
	var out []any
	err := g.IterAll(dataset, q, func(page map[string]any) bool {
		out = append(out, AsList(page["items"])...)
		return true
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
