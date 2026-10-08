// 自建后端（Elysia）传输层 + 公开读接口。
//
// 只覆盖无需登录的 Query：评论 / 构筑 / 攻略 / 时间轴 / 榜单 / 脚本 /
// MOD / DPS / gameData*。Mutation、admin、shop 私有、my* 一律不实现。
package dob

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout 默认请求超时（秒）。
const DefaultTimeout = 30 * time.Second

// BackendClient 是自建后端客户端，默认线上端点；本地联调传 http://localhost:8887。
type BackendClient struct {
	BaseURL string
	Timeout time.Duration
	client  *http.Client
}

// NewBackendClient 构造客户端（baseURL 为空用线上端点，timeout<=0 用默认）。
func NewBackendClient(baseURL string, timeout time.Duration) *BackendClient {
	if baseURL == "" {
		baseURL = "https://api.dna-builder.cn"
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &BackendClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// GraphQLURL 端点地址。
func (b *BackendClient) GraphQLURL() string { return b.BaseURL + "/graphql" }

func (b *BackendClient) postJSON(url string, payload any) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &DobApiError{Message: err.Error(), Code: "encode_error"}
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, &DobApiError{Message: err.Error(), Code: "encode_error"}
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, httpErrorf(0, "http_error", "连接失败: %s (%v)", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httpErrorf(0, "http_error", "连接失败: %s (%v)", url, err)
	}
	var parsed any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&parsed); err != nil {
		parsed = map[string]any{"message": string(raw)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httpErrorf(resp.StatusCode, "http_error", "HTTP %d: %s", resp.StatusCode, url)
	}
	m, _ := parsed.(map[string]any)
	if m == nil {
		m = map[string]any{"message": string(raw)}
	}
	return m, nil
}

// GraphQL 裸调用，errors 非空时抛 *DobGraphQLError。
func (b *BackendClient) GraphQL(query string, variables map[string]any) (map[string]any, error) {
	if variables == nil {
		variables = map[string]any{}
	}
	body, err := b.postJSON(b.GraphQLURL(), map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, err
	}
	if errs, ok := body["errors"].([]any); ok && len(errs) > 0 {
		msg := "graphql error"
		if first, ok := errs[0].(map[string]any); ok {
			if s, ok := first["message"].(string); ok {
				msg = s
			}
		} else if s, ok := errs[0].(string); ok {
			msg = s
		}
		return nil, gqlErrorf(msg, errs)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func gqlList(data map[string]any, key string) []any {
	if l, ok := data[key].([]any); ok {
		return l
	}
	return []any{}
}

func gqlMap(data map[string]any, key string) map[string]any {
	if m, ok := data[key].(map[string]any); ok {
		return m
	}
	return nil
}

func gqlInt(data map[string]any, key string) int {
	return int(Num(data[key]))
}

func optStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func optInt(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

// ---- 评论（读） ----

// Comments 评论列表。
func (b *BackendClient) Comments(targetID string, limit, offset int) ([]any, error) {
	data, err := b.GraphQL(
		"query($t:String!,$l:Int,$o:Int){comments(targetId:$t,limit:$l,offset:$o){id targetId content createdAt user{id name}}}",
		map[string]any{"t": targetID, "l": limit, "o": offset},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "comments"), nil
}

// CommentsCount 评论数。
func (b *BackendClient) CommentsCount(targetID string) (int, error) {
	data, err := b.GraphQL("query($t:String!){commentsCount(targetId:$t)}", map[string]any{"t": targetID})
	if err != nil {
		return 0, err
	}
	return gqlInt(data, "commentsCount"), nil
}

// ---- 构筑（读） ----

// Builds 构筑列表。
func (b *BackendClient) Builds(search *string, charID *int, userID *string, limit, offset int, sortBy string) ([]any, error) {
	data, err := b.GraphQL(
		"query($s:String,$c:Int,$u:String,$l:Int,$o:Int,$sb:String)"+
			"{builds(search:$s,charId:$c,userId:$u,limit:$l,offset:$o,sortBy:$sb)"+
			"{id title desc charId userId views likes isRecommended isPinned createdAt updateAt targetValue charSettings}}",
		map[string]any{"s": optStr(search), "c": optInt(charID), "u": optStr(userID), "l": limit, "o": offset, "sb": sortBy},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "builds"), nil
}

// BuildsCount 构筑数。
func (b *BackendClient) BuildsCount(search *string, charID *int) (int, error) {
	data, err := b.GraphQL("query($s:String,$c:Int){buildsCount(search:$s,charId:$c)}",
		map[string]any{"s": optStr(search), "c": optInt(charID)})
	if err != nil {
		return 0, err
	}
	return gqlInt(data, "buildsCount"), nil
}

// Build 单个构筑（含 charSettings）。
func (b *BackendClient) Build(buildID string) (map[string]any, error) {
	data, err := b.GraphQL(
		"query($id:String!){build(id:$id){id title desc charId charSettings userId views likes createdAt updateAt targetValue}}",
		map[string]any{"id": buildID},
	)
	if err != nil {
		return nil, err
	}
	return gqlMap(data, "build"), nil
}

// RecommendedBuilds 推荐构筑。
func (b *BackendClient) RecommendedBuilds(limit int) ([]any, error) {
	data, err := b.GraphQL("query($l:Int){recommendedBuilds(limit:$l){id title charId likes views}}", map[string]any{"l": limit})
	if err != nil {
		return nil, err
	}
	return gqlList(data, "recommendedBuilds"), nil
}

// TrendingBuilds 热门构筑。
func (b *BackendClient) TrendingBuilds(limit int) ([]any, error) {
	data, err := b.GraphQL("query($l:Int){trendingBuilds(limit:$l){id title charId likes views}}", map[string]any{"l": limit})
	if err != nil {
		return nil, err
	}
	return gqlList(data, "trendingBuilds"), nil
}

// ---- 攻略（读） ----

// Guides 攻略列表。
func (b *BackendClient) Guides(search, type_ *string, charID *int, userID *string, limit, offset int) ([]any, error) {
	data, err := b.GraphQL(
		"query($s:String,$t:String,$c:Int,$u:String,$l:Int,$o:Int)"+
			"{guides(search:$s,type:$t,charId:$c,userId:$u,limit:$l,offset:$o){id title type charId views likes createdAt updateAt}}",
		map[string]any{"s": optStr(search), "t": optStr(type_), "c": optInt(charID), "u": optStr(userID), "l": limit, "o": offset},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "guides"), nil
}

// Guide 单个攻略。
func (b *BackendClient) Guide(guideID string) (map[string]any, error) {
	data, err := b.GraphQL(
		"query($id:String!){guide(id:$id){id title type content images charId buildId views likes createdAt updateAt}}",
		map[string]any{"id": guideID},
	)
	if err != nil {
		return nil, err
	}
	return gqlMap(data, "guide"), nil
}

// ---- 时间轴（读） ----

// Timelines 时间轴列表。
func (b *BackendClient) Timelines(search *string, charID *int, userID *string, limit, offset int, sortBy string) ([]any, error) {
	data, err := b.GraphQL(
		"query($s:String,$c:Int,$u:String,$l:Int,$o:Int,$sb:String)"+
			"{timelines(search:$s,charId:$c,userId:$u,limit:$l,offset:$o,sortBy:$sb){id title charId views likes createdAt updateAt}}",
		map[string]any{"s": optStr(search), "c": optInt(charID), "u": optStr(userID), "l": limit, "o": offset, "sb": sortBy},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "timelines"), nil
}

// Timeline 单个时间轴。
func (b *BackendClient) Timeline(timelineID string) (map[string]any, error) {
	data, err := b.GraphQL(
		"query($id:String!){timeline(id:$id){id title charId views likes createdAt updateAt}}",
		map[string]any{"id": timelineID},
	)
	if err != nil {
		return nil, err
	}
	return gqlMap(data, "timeline"), nil
}

// ---- 榜单（读） ----

// RankingLists 榜单列表。
func (b *BackendClient) RankingLists() ([]any, error) {
	data, err := b.GraphQL("{rankingLists{id name desc createdAt updateAt}}", nil)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "rankingLists"), nil
}

// RankingList 单个榜单。
func (b *BackendClient) RankingList(listID string) (map[string]any, error) {
	data, err := b.GraphQL(
		"query($id:String!){rankingList(id:$id){id name desc items{id charId buildId sortOrder}}}",
		map[string]any{"id": listID},
	)
	if err != nil {
		return nil, err
	}
	return gqlMap(data, "rankingList"), nil
}

// RankingListItems 榜单条目。
func (b *BackendClient) RankingListItems(rankingListID string) ([]any, error) {
	data, err := b.GraphQL(
		"query($id:String!){rankingListItems(rankingListId:$id){id charId buildId sortOrder}}",
		map[string]any{"id": rankingListID},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "rankingListItems"), nil
}

// ---- 脚本 / MOD / DPS（读） ----

// Scripts 脚本列表。
func (b *BackendClient) Scripts(search, category, userID *string, limit, offset int) ([]any, error) {
	data, err := b.GraphQL(
		"query($s:String,$c:String,$u:String,$l:Int,$o:Int)"+
			"{scripts(search:$s,category:$c,userId:$u,limit:$l,offset:$o){id title category createdAt updateAt}}",
		map[string]any{"s": optStr(search), "c": optStr(category), "u": optStr(userID), "l": limit, "o": offset},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "scripts"), nil
}

// Script 单个脚本。
func (b *BackendClient) Script(scriptID string, preview bool) (map[string]any, error) {
	data, err := b.GraphQL(
		"query($id:String!,$p:Boolean){script(id:$id,preview:$p){id title category content}}",
		map[string]any{"id": scriptID, "p": preview},
	)
	if err != nil {
		return nil, err
	}
	return gqlMap(data, "script"), nil
}

// GameMods 游戏 MOD 列表。
func (b *BackendClient) GameMods(search, category, entity *string, limit, offset int, sortBy *string) ([]any, error) {
	data, err := b.GraphQL(
		"query($s:String,$c:String,$e:String,$l:Int,$o:Int,$sb:String)"+
			"{gameMods(search:$s,category:$c,entity:$e,limit:$l,offset:$o,sortBy:$sb){id title category entity status createdAt updateAt}}",
		map[string]any{"s": optStr(search), "c": optStr(category), "e": optStr(entity), "l": limit, "o": offset, "sb": optStr(sortBy)},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "gameMods"), nil
}

// GameMod 单个游戏 MOD。
func (b *BackendClient) GameMod(modID string) (map[string]any, error) {
	data, err := b.GraphQL("query($id:String!){gameMod(id:$id){id title category entity status}}", map[string]any{"id": modID})
	if err != nil {
		return nil, err
	}
	return gqlMap(data, "gameMod"), nil
}

// DPSList DPS 记录列表。
func (b *BackendClient) DPSList(charID *int, buildID, timelineID *string, limit, offset int, sortBy string) ([]any, error) {
	data, err := b.GraphQL(
		"query($c:Int,$b:String,$t:String,$l:Int,$o:Int,$sb:String)"+
			"{dpsList(charId:$c,buildId:$b,timelineId:$t,limit:$l,offset:$o,sortBy:$sb){id charId buildId dpsValue createdAt}}",
		map[string]any{"c": optInt(charID), "b": optStr(buildID), "t": optStr(timelineID), "l": limit, "o": offset, "sb": sortBy},
	)
	if err != nil {
		return nil, err
	}
	return gqlList(data, "dpsList"), nil
}
