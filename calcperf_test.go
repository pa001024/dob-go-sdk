// fixture BD 数值批量计算性能基线（纯本地，无网络）。
package dob

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// 病态预算（只防挂死，不做机器相关的精密门控）。
const (
	onlineBatchBudget = 5 * time.Second
	shardBatchBudget  = 10 * time.Second
	onlineRounds      = 5
	shardRounds       = 3
)

func TestCalcPerfOnlineBatch(t *testing.T) {
	tables := onlineTables(t)
	expected := loadGolden(t, "online_expected.json").(map[string]any)["expected"].(map[string]any)
	dir := filepath.Join(goldenDir(t), "online")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	var samples []time.Duration
	for r := 0; r < onlineRounds; r++ {
		start := time.Now()
		for _, file := range files {
			payload := loadGolden(t, "online/"+file).(map[string]any)
			key := strings.TrimSuffix(file, ".json")
			want := expected[key].(map[string]any)
			settings, _ := payload["settings"].(map[string]any)
			state, err := BuildState(int(Num(payload["charId"])), settings, tables, BuildOptions{})
			if err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(state, tables)
			skipped := dotDependent(settings)
			tf, _ := settings["targetFunction"].(string)
			if !skipped[tf] {
				if tw, ok := want["target"]; ok && IsNum(tw) {
					if got := engine.Calculate(""); !isClose(got, Num(tw)) {
						t.Fatalf("%s target: mine=%v ref=%v", file, got, Num(tw))
					}
				}
			}
			_ = engine.CalculateWeaponAttributes(nil, false, false)
		}
		samples = append(samples, time.Since(start))
	}
	b := best(samples)
	t.Logf("[perf] online batch x%d 装配+目标+全量属性 best-of-%d: %v", len(files), onlineRounds, b)
	if b > onlineBatchBudget {
		t.Errorf("online 批量超预算: %v", b)
	}
}

func TestCalcPerfShardBatch(t *testing.T) {
	bd := loadGolden(t, "shardBuild1_bd.json").(map[string]any)
	data := loadGolden(t, "shardBuild1_data.json").(map[string]any)
	expr := loadGolden(t, "shardBuild1_expr.json").(map[string]any)
	tables := tablesFor(t, data)
	settings, _ := bd["settings"].(map[string]any)
	var samples []time.Duration
	for r := 0; r < shardRounds; r++ {
		start := time.Now()
		state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{})
		if err != nil {
			t.Fatal(err)
		}
		engine := NewEngine(state, tables)
		for _, c := range expr["cases"].([]any) {
			cm := c.(map[string]any)
			name, _ := cm["expr"].(string)
			if got := engine.Calculate(name); !isClose(got, Num(cm["ts"])) {
				t.Fatalf("%s: mine=%v ref=%v", name, got, Num(cm["ts"]))
			}
		}
		_ = engine.CalculateWeaponAttributes(nil, false, false)
		samples = append(samples, time.Since(start))
	}
	b := best(samples)
	t.Logf("[perf] shard batch 42expr+attrs best-of-%d: %v", shardRounds, b)
	if b > shardBatchBudget {
		t.Errorf("shard 批量超预算: %v", b)
	}
}
