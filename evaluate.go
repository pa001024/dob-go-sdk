// 纯表达式求值：与 TS 侧 CharBuild.evaluateAST 在纯属性模式下逐项对齐。
package dob

import "math"

// DamageBranchMembers 是已知的伤害分支成员名（命中时回退期望值）。
var DamageBranchMembers = map[string]bool{
	"N": true, "物理": true, "元素": true, "暴击": true, "未暴击": true,
	"触发": true, "未触发": true, "暴击触发": true, "触发暴击": true,
	"未触发暴击": true, "暴击未触发": true, "触发未暴击": true,
	"未暴击触发": true, "未暴击未触发": true, "未触发未暴击": true,
}

// BoostMultiplier 昂扬乘区（NaN 按 Python min/max 口径得钳制值 1）。
func BoostMultiplier(attrs map[string]any, hpPercent float64) float64 {
	hp := pyMin(1, hpPercent)
	hp = pyMax(0, hp)
	return 1 + Num(attrs["昂扬"])*hp
}

// DesperateMultiplier 背水乘区。
func DesperateMultiplier(attrs map[string]any, hpPercent float64) float64 {
	hp := pyMin(1, hpPercent)
	hp = pyMax(0.25, hp)
	return 1 + 4*Num(attrs["背水"])*(1-hp)*(1.5-hp)
}

// pyMin/pyMax 复刻 Python 内建 min/max（NaN 不传播，只按 < / > 比较）。
func pyMin(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func pyMax(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

func lookupAttr(attrs map[string]any, name, namespace string, panels map[string]any) float64 {
	if namespace != "" {
		if panel, ok := panels[namespace].(map[string]any); ok {
			if v, ok := panel[name]; ok {
				if _, isMap := v.(map[string]any); !isMap {
					if f := Num(v); f != 0 {
						return f
					}
				}
			}
		}
		key := namespace + "::" + name
		if v, ok := attrs[key]; ok {
			if _, isMap := v.(map[string]any); !isMap {
				if f := Num(v); f != 0 {
					return f
				}
			}
		}
		if v, ok := attrs[name]; ok {
			if _, isMap := v.(map[string]any); !isMap {
				return Num(v)
			}
		}
		return 0
	}
	if v, ok := attrs[name]; ok {
		if _, isMap := v.(map[string]any); !isMap {
			if f := Num(v); f != 0 {
				return f
			}
		}
	}
	if selected, ok := attrs["weapon"].(map[string]any); ok {
		if v, ok := selected[name]; ok {
			if _, isMap := v.(map[string]any); !isMap {
				return Num(v)
			}
		}
	}
	return 0
}

// CustomFunc 自定义函数定义。
type CustomFunc struct {
	Params []string
	Body   string
}

// Evaluator 是表达式求值器（可复用 AST 缓存）。
type Evaluator struct {
	Attrs           map[string]any
	Scope           map[string]any
	CustomVariables map[string]string
	CustomFunctions map[string]CustomFunc
	WeaponPanels    map[string]any
	DamageCtx       *DamageContext
	cache           map[string]*Node
}

// NewEvaluator 构造求值器。
func NewEvaluator(attrs map[string]any, scope map[string]any, vars map[string]string, funcs map[string]CustomFunc, panels map[string]any, ctx *DamageContext) *Evaluator {
	if attrs == nil {
		attrs = map[string]any{}
	}
	return &Evaluator{Attrs: attrs, Scope: scope, CustomVariables: vars, CustomFunctions: funcs, WeaponPanels: panels, DamageCtx: ctx, cache: map[string]*Node{}}
}

// Evaluate 求值表达式（字符串或 AST 节点），返回 float64。
func Evaluate(expr any, attrs map[string]any, scope map[string]any, vars map[string]string, funcs map[string]CustomFunc, panels map[string]any, ctx *DamageContext) float64 {
	ev := NewEvaluator(attrs, scope, vars, funcs, panels, ctx)
	return ev.Eval(expr, nil, nil, nil, nil)
}

// Eval 求值（overlay 临时属性，resolvingVars/Funcs 环引用集合）。
func (e *Evaluator) Eval(expr any, overlay map[string]any, scope map[string]any, resolvingVars, resolvingFuncs map[string]bool) float64 {
	var node *Node
	switch t := expr.(type) {
	case string:
		cached, ok := e.cache["expr:"+t]
		if !ok {
			n, err := ParseAST(t, nil)
			if err != nil {
				return 0
			}
			e.cache["expr:"+t] = n
			cached = n
		}
		node = cached
	case *Node:
		node = t
	default:
		return 0
	}
	if overlay == nil {
		overlay = map[string]any{}
	}
	if scope == nil {
		scope = e.Scope
		if scope == nil {
			scope = map[string]any{}
		}
	}
	if resolvingVars == nil {
		resolvingVars = map[string]bool{}
	}
	if resolvingFuncs == nil {
		resolvingFuncs = map[string]bool{}
	}
	result := e.ev(node, overlay, scope, resolvingVars, resolvingFuncs)
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0
	}
	return result
}

func (e *Evaluator) ev(n *Node, overlay, scope map[string]any, rv, rf map[string]bool) float64 {
	switch n.Type {
	case NodeNumber:
		return n.Value
	case NodeBinary:
		left := e.ev(n.Left, overlay, scope, rv, rf)
		right := e.ev(n.Right, overlay, scope, rv, rf)
		switch n.Operator {
		case "+":
			return left + right
		case "-":
			return left - right
		case "*":
			return left * right
		case "/":
			if right != 0 {
				return left / right
			}
			return 0
		case "%":
			if right != 0 {
				return math.Mod(left, right)
			}
			return 0
		case "//":
			if right != 0 {
				return math.Floor(left / right)
			}
			return 0
		}
		panic(&AstError{Msg: "未知的二元运算符: " + n.Operator})
	case NodeUnary:
		arg := e.ev(n.Argument, overlay, scope, rv, rf)
		if n.Operator == "+" {
			return arg
		}
		if n.Operator == "-" {
			return -arg
		}
		panic(&AstError{Msg: "未知的一元运算符: " + n.Operator})
	case NodeProperty:
		return e.evProperty(n, overlay, scope, rv, rf)
	case NodeFunction:
		args := make([]float64, 0, len(n.Args))
		for _, a := range n.Args {
			args = append(args, e.ev(a, overlay, scope, rv, rf))
		}
		return e.evFunction(n.Name, args, overlay, scope, rv, rf)
	case NodeMember:
		return e.evMember(n, overlay, scope, rv, rf)
	case NodeTemp:
		merged := CloneMap(overlay)
		for _, item := range n.Attrs {
			merged[item.Name] = Num(merged[item.Name]) + e.ev(item.Value, overlay, scope, rv, rf)
		}
		return e.ev(n.Target, merged, scope, rv, rf)
	}
	panic(&AstError{Msg: "未知的 AST 节点"})
}

func (e *Evaluator) evCb(nd *Node, ov map[string]any, sc map[string]any) float64 {
	if ov == nil {
		ov = map[string]any{}
	}
	if sc == nil {
		sc = map[string]any{}
	}
	return e.ev(nd, ov, sc, map[string]bool{}, map[string]bool{})
}

func (e *Evaluator) ctxSafeOf(n *Node) string {
	if e.DamageCtx != nil && n.Namespace != "" {
		return e.DamageCtx.ResolveSkillSafe(n.Namespace)
	}
	return ""
}

func (e *Evaluator) mergeTemp(attrs []TempAttr, inherited, scope map[string]any, rv, rf map[string]bool) map[string]any {
	merged := CloneMap(inherited)
	if merged == nil {
		merged = map[string]any{}
	}
	for _, item := range attrs {
		merged[item.Name] = Num(merged[item.Name]) + e.ev(item.Value, inherited, scope, rv, rf)
	}
	return merged
}

func (e *Evaluator) propContext(node *Node, inherited, scope map[string]any, rv, rf map[string]bool) (*Node, map[string]any, bool) {
	if node.Type == NodeProperty {
		return node, inherited, true
	}
	if node.Type != NodeTemp {
		return nil, nil, false
	}
	return e.propContext(node.Target, e.mergeTemp(node.Attrs, inherited, scope, rv, rf), scope, rv, rf)
}

func (e *Evaluator) evIdentity(n *Node, overlay, scope map[string]any, rv, rf map[string]bool) float64 {
	name, ns := n.Name, n.Namespace
	force := n.ForceAttr
	if !force && ns == "" {
		if v, ok := scope[name]; ok {
			return Num(v)
		}
	}
	if !force && ns == "" && e.CustomVariables != nil {
		if body, ok := e.CustomVariables[name]; ok {
			if rv[name] {
				return 0
			}
			key := "var:" + body
			sub, ok := e.cache[key]
			if !ok {
				var err error
				sub, err = ParseAST(body, nil)
				if err != nil {
					return 0
				}
				e.cache[key] = sub
			}
			nrv := cloneSet(rv)
			nrv[name] = true
			value := e.ev(sub, overlay, scope, nrv, rf)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return 0
			}
			return value
		}
	}
	ctxSafe := e.ctxSafeOf(n)
	if e.DamageCtx != nil {
		if !force {
			sval := e.DamageCtx.EvaluateSkill(name, ns, overlayOrNil(overlay), ctxSafe, e.evCb)
			if sval != 0 {
				return sval
			}
		}
		merged := e.DamageCtx.SummonAttrs(ns, overlayOrNil(overlay), name, ctxSafe)
		if ns != "" {
			if panel, ok := e.WeaponPanels[ns].(map[string]any); ok {
				if v, ok := panel[name]; ok {
					if f := Num(v); f != 0 {
						return f
					}
				}
			}
			if v, ok := merged[name]; ok && IsNum(v) {
				return Num(v)
			}
			return 0
		}
		if v, ok := merged[name]; ok && IsNum(v) {
			if f := Num(v); f != 0 {
				return f
			}
		}
		if selected, ok := e.Attrs["weapon"].(map[string]any); ok {
			if v, ok := selected[name]; ok && IsNum(v) {
				return Num(v)
			}
		}
		return 0
	}
	if overlay != nil {
		if _, ok := overlay[name]; ok {
			if raw, ok := e.Attrs[name]; ok && IsNum(raw) {
				return Num(raw) + Num(overlay[name])
			}
			if _, ok := e.Attrs[name]; !ok {
				return Num(overlay[name])
			}
		}
	}
	return lookupAttr(e.Attrs, name, ns, e.WeaponPanels)
}

func (e *Evaluator) evProperty(n *Node, overlay, scope map[string]any, rv, rf map[string]bool) float64 {
	name, ns := n.Name, n.Namespace
	force := n.ForceAttr
	if ns == "" && !force {
		if v, ok := scope[name]; ok {
			return Num(v)
		}
	}
	value := e.evIdentity(n, overlay, scope, rv, rf)
	if e.DamageCtx != nil && !force {
		if ns == "" && e.CustomVariables != nil {
			if _, ok := e.CustomVariables[name]; ok {
				return value
			}
		}
		sval := e.DamageCtx.EvaluateSkill(name, ns, overlayOrNil(overlay), e.ctxSafeOf(n), e.evCb)
		if sval != 0 && e.DamageCtx.IsDamageSkillField(name, ns, e.ctxSafeOf(n)) {
			damage := e.DamageCtx.GetDamage(ns, name, overlayOrNil(overlay), e.ctxSafeOf(n))
			return value * Num(damage["expectedDamage"])
		}
	}
	return value
}

func (e *Evaluator) evFunction(name string, args []float64, overlay, scope map[string]any, rv, rf map[string]bool) float64 {
	switch name {
	case "min":
		if len(args) == 0 {
			panic(&AstError{Msg: "min 需要参数"})
		}
		m := args[0]
		for _, v := range args[1:] {
			if v < m {
				m = v
			}
		}
		return m
	case "max":
		if len(args) == 0 {
			panic(&AstError{Msg: "max 需要参数"})
		}
		m := args[0]
		for _, v := range args[1:] {
			if v > m {
				m = v
			}
		}
		return m
	case "floor":
		return math.Floor(args[0])
	case "ceil":
		return math.Ceil(args[0])
	case "or":
		for _, v := range args {
			if v != 0 {
				return v
			}
		}
		if len(args) > 0 {
			return args[len(args)-1]
		}
		return 0
	case "log":
		return math.Log(args[0])
	case "power":
		return math.Pow(args[0], args[1])
	case "hp":
		hp := 1.0
		if len(args) > 0 {
			hp = args[0]
		}
		return DesperateMultiplier(e.Attrs, hp) * BoostMultiplier(e.Attrs, hp)
	}
	if e.CustomFunctions != nil {
		if def, ok := e.CustomFunctions[name]; ok {
			if rf[name] {
				return 0
			}
			if len(def.Params) != len(args) {
				panic(&AstError{Msg: "函数参数数量不匹配: " + name})
			}
			key := "func:" + def.Body
			sub, ok := e.cache[key]
			if !ok {
				var err error
				sub, err = ParseAST(def.Body, nil)
				if err != nil {
					return 0
				}
				e.cache[key] = sub
			}
			nextScope := CloneMap(scope)
			if nextScope == nil {
				nextScope = map[string]any{}
			}
			for i, p := range def.Params {
				nextScope[p] = args[i]
			}
			nrf := cloneSet(rf)
			nrf[name] = true
			return e.ev(sub, overlay, nextScope, rv, nrf)
		}
	}
	panic(&AstError{Msg: "未知的函数: " + name})
}

func (e *Evaluator) evMember(n *Node, overlay, scope map[string]any, rv, rf map[string]bool) float64 {
	obj, member := n.Object, n.Property
	if e.DamageCtx != nil {
		if prop, mergedTemp, ok := e.propContext(obj, overlay, scope, rv, rf); ok {
			objectValue := e.evIdentity(prop, mergedTemp, scope, rv, rf)
			ctxSafe := e.ctxSafeOf(prop)
			if prop.ForceAttr || !e.DamageCtx.IsDamageSkillField(prop.Name, prop.Namespace, ctxSafe) {
				return objectValue
			}
			damage := e.DamageCtx.GetDamage(prop.Namespace, prop.Name, mergedTempOrNil(mergedTemp), ctxSafe)
			return objectValue * e.DamageCtx.DamageMember(damage, member)
		}
		objectValue := e.ev(obj, overlay, scope, rv, rf)
		damage := e.DamageCtx.GetDamage("", "", overlayOrNil(overlay), "")
		return objectValue * e.DamageCtx.DamageMember(damage, member)
	}
	return e.legacyMemberValue(n, overlay, scope, rv, rf)
}

func (e *Evaluator) legacyMemberValue(n *Node, overlay, scope map[string]any, rv, rf map[string]bool) float64 {
	obj, member := n.Object, n.Property
	var base float64
	var container map[string]any
	if obj.Type == NodeProperty {
		base = e.evIdentity(obj, overlay, scope, rv, rf)
		key := obj.Name
		if obj.Namespace != "" {
			key = obj.Namespace + "::" + obj.Name
		}
		raw, ok := overlay[key]
		if !ok {
			raw, ok = e.Attrs[key]
		}
		if !ok {
			raw = e.Attrs[obj.Name]
		}
		if m, ok := raw.(map[string]any); ok {
			container = m
		}
	} else {
		base = e.ev(obj, overlay, scope, rv, rf)
	}
	if container != nil {
		if v, ok := container[member]; ok {
			return Num(v)
		}
	}
	if DamageBranchMembers[member] {
		return base
	}
	return 0
}

func overlayOrNil(overlay map[string]any) map[string]any {
	if len(overlay) == 0 {
		return nil
	}
	return overlay
}

func mergedTempOrNil(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	return m
}

func cloneSet(s map[string]bool) map[string]bool {
	out := make(map[string]bool, len(s)+1)
	for k, v := range s {
		out[k] = v
	}
	return out
}
