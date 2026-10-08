// BUFF code 语料回归：14 段真实 code 可解析、可执行。
package dob

import (
	"math"
	"testing"
)

func jsSandbox() map[string]any {
	panel := map[string]any{"攻击": 100.0, "暴击": 1.0, "暴伤": 2.0, "触发": 1.0, "增伤": 0.5}
	base := map[string]any{"基础攻击": 200.0, "基础暴击": 0.2, "基础暴伤": 2.0, "基础触发": 0.5}
	clone := func() map[string]any {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		return m
	}
	pclone := func() map[string]any {
		m := map[string]any{}
		for k, v := range panel {
			m[k] = v
		}
		return m
	}
	return map[string]any{
		"攻击": 1000.0, "生命": 5000.0, "神智": 300.0, "技能威力": 2.0, "技能范围": 1.5,
		"技能耐久": 1.2, "昂扬": 0.5, "属性穿透": 0.1, "技能无视防御": 0.0,
		"充盈威力": 1.0, "召唤物攻击速度": 0.5, "召唤物独立增伤": 0.0, "增伤": 0.5,
		"char": clone(), "weapon": clone(), "meleeWeapon": clone(), "rangedWeapon": clone(), "skillWeapon": clone(),
		"weaponAttr": pclone(), "meleeWeaponAttr": pclone(), "rangedWeaponAttr": pclone(), "skillWeaponAttr": pclone(),
		"enemy":      map[string]any{"等级": 80},
		"charMods":   map[string]any{"攻击": 1.0, "暴击": 0.5, "触发": 0.5},
		"meleeMods":  map[string]any{"暴击": 0.5, "暴伤": 0.5, "触发": 0.5},
		"rangedMods": map[string]any{}, "skillMods": map[string]any{},
	}
}

func TestJsCodeCorpus(t *testing.T) {
	corpus, ok := loadGolden(t, "jscode_corpus.json").([]any)
	if !ok || len(corpus) != 14 {
		t.Fatalf("语料缺失: %v", corpus)
	}
	for _, entry := range corpus {
		em, _ := entry.(map[string]any)
		name, _ := em["name"].(string)
		code, _ := em["code"].(string)
		t.Run(name, func(t *testing.T) {
			if _, err := JsParseProgram(code); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("执行失败: %v", r)
					}
				}()
				RunJsCode(code, jsSandbox())
			}()
		})
	}
}

func TestVarMultiDeclarators(t *testing.T) {
	sandbox := map[string]any{"x": 0.0}
	RunJsCode("var c=3,t=4;if(t>0){x+=c+t}", sandbox)
	if Num(sandbox["x"]) != 7 {
		t.Errorf("got %v", sandbox["x"])
	}
}

func TestMathNaNPropagation(t *testing.T) {
	sandbox := map[string]any{"x": 0.0, "u": math.NaN()}
	RunJsCode("x=Math.min(1,u)+Math.max(1,2)", sandbox)
	if !math.IsNaN(Num(sandbox["x"])) {
		t.Errorf("got %v", sandbox["x"])
	}
}
