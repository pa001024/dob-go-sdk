// 真实数据包加载性能基线（纯本地，无网络）。
package dob

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 钉住的数据包版本（条数锚点与之绑定）。
const pinVersion = "1.6.208.6"

var pinCounts = map[string]int{
	"chars": 33, "mods": 586, "buffs": 187, "effects": 65,
	"weapons": 71, "pets": 152, "pet_entries": 73, "monsters": 393,
}

// 病态预算（只防挂死，不做机器相关的精密门控）。
const (
	activateBudget   = 60 * time.Second
	loadTablesBudget = 60 * time.Second
	perfRounds       = 3
)

func pinPackDir(t *testing.T) string {
	t.Helper()
	// sdk/go → 仓库根：../..
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mock := filepath.Join(root, "mock", "data-pack", pinVersion+".zip")
	if _, err := os.Stat(mock); err != nil {
		t.Skipf("mock 包缺失: %v", err)
	}
	cache, err := os.MkdirTemp("", "dob-perf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(cache) })
	destDir := filepath.Join(cache, pinVersion)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(destDir, "package.zip")
	if err := os.Link(mock, dest); err != nil {
		raw, err := os.ReadFile(mock)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cache
}

func best(durations []time.Duration) time.Duration {
	// 环境时钟粗粒度时亚毫秒样本可能为 0：取最小非零值（全零则返回 0）。
	m := time.Duration(0)
	for _, d := range durations {
		if d <= 0 {
			continue
		}
		if m == 0 || d < m {
			m = d
		}
	}
	return m
}

func TestDatapackPerf(t *testing.T) {
	cache := pinPackDir(t)
	pack := NewDataPackStore(cache, "", "-")
	var actTimes []time.Duration
	var manifest map[string]any
	for i := 0; i < perfRounds; i++ {
		start := time.Now()
		m, err := pack.Activate(pinVersion)
		if err != nil {
			t.Fatal(err)
		}
		actTimes = append(actTimes, time.Since(start))
		manifest = m
	}
	if S(manifest, "version") != pinVersion {
		t.Errorf("manifest 版本: %v", S(manifest, "version"))
	}
	bestAct := best(actTimes)
	t.Logf("[perf] datapack activate(%s) best-of-%d: %v", pinVersion, perfRounds, bestAct)
	if bestAct > activateBudget {
		t.Errorf("activate 超预算: %v", bestAct)
	}
	var loadTimes []time.Duration
	var tables *GameDataTables
	for i := 0; i < perfRounds; i++ {
		start := time.Now()
		tb, err := pack.LoadTables()
		if err != nil {
			t.Fatal(err)
		}
		loadTimes = append(loadTimes, time.Since(start))
		tables = tb
	}
	counts := map[string]int{
		"chars": len(tables.Chars), "mods": len(tables.Mods), "buffs": len(tables.Buffs),
		"effects": len(tables.Effects), "weapons": len(tables.Weapons), "pets": len(tables.Pets),
		"pet_entries": len(tables.PetEntries), "monsters": len(tables.Monsters),
	}
	for k, want := range pinCounts {
		if counts[k] != want {
			t.Errorf("表 %s 条数: mine=%d ref=%d", k, counts[k], want)
		}
	}
	bestLoad := best(loadTimes)
	t.Logf("[perf] datapack load_tables(%s) best-of-%d: %v counts=%v", pinVersion, perfRounds, bestLoad, counts)
	if bestLoad > loadTablesBudget {
		t.Errorf("load_tables 超预算: %v", bestLoad)
	}
}
