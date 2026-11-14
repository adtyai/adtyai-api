package billingexpr

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/file"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
)

var errNonFiniteExpr = errors.New("expr result must be finite and non-negative")

type decimalEvalEnv struct {
	src     string
	vars    map[string]decimal.Decimal
	trace   *TraceResult
	headers map[string]string
	body    []byte
	usage   map[string]any
}

func evalBillingExpr(node ast.Node, src string, env *decimalEvalEnv) (decimal.Decimal, error) {
	out, err := evalNode(node, env)
	if err != nil {
		return decimal.Zero, err
	}
	cost, ok := valueAsDecimal(out)
	if !ok {
		return decimal.Zero, fmt.Errorf("expr result is %T, want a finite non-negative number", out)
	}
	if !decimalFiniteNonNegative(cost) {
		return decimal.Zero, errNonFiniteExpr
	}
	return cost, nil
}

func evalNode(node ast.Node, env *decimalEvalEnv) (any, error) {
	if node == nil {
		return nil, fmt.Errorf("expr run error: missing node")
	}
	switch n := node.(type) {
	case *ast.NilNode:
		return nil, nil
	case *ast.BoolNode:
		return n.Value, nil
	case *ast.StringNode:
		return n.Value, nil
	case *ast.IntegerNode:
		return decimal.NewFromInt(int64(n.Value)), nil
	case *ast.FloatNode:
		return decimalFromFloatNode(env.src, n), nil
	case *ast.IdentifierNode:
		return evalIdentifier(n.Value, env)
	case *ast.UnaryNode:
		return evalUnary(n, env)
	case *ast.BinaryNode:
		return evalBinary(n, env)
	case *ast.ConditionalNode:
		cond, err := evalNode(n.Cond, env)
		if err != nil {
			return nil, err
		}
		ok, err := truthy(cond)
		if err != nil {
			return nil, err
		}
		if ok {
			return evalNode(n.Exp1, env)
		}
		return evalNode(n.Exp2, env)
	case *ast.CallNode:
		return evalCall(n, env)
	case *ast.BuiltinNode:
		return evalBuiltin(n, env)
	default:
		return nil, fmt.Errorf("expr run error: unsupported expression node %T", node)
	}
}

func evalIdentifier(name string, env *decimalEvalEnv) (any, error) {
	if d, ok := env.vars[name]; ok {
		return d, nil
	}
	switch name {
	case requestRuleTraceFunction, requestRuleTraceIntFunction, "tier", "fixed", "header", "param", "u", "has", "hour", "minute", "weekday", "month", "day", "max", "min", "abs", "ceil", "floor":
		return nil, fmt.Errorf("expr run error: identifier %q cannot be used as a value", name)
	default:
		return nil, fmt.Errorf("expr run error: unknown identifier %q", name)
	}
}

func evalUnary(n *ast.UnaryNode, env *decimalEvalEnv) (any, error) {
	v, err := evalNode(n.Node, env)
	if err != nil {
		return nil, err
	}
	switch n.Operator {
	case "!", "not":
		ok, err := truthy(v)
		if err != nil {
			return nil, err
		}
		return !ok, nil
	case "+", "-":
		d, ok := valueAsDecimal(v)
		if !ok {
			return nil, fmt.Errorf("expr run error: unary %s requires a number, got %T", n.Operator, v)
		}
		if n.Operator == "-" {
			return d.Neg(), nil
		}
		return d, nil
	default:
		return nil, fmt.Errorf("expr run error: unsupported unary operator %q", n.Operator)
	}
}

func evalBinary(n *ast.BinaryNode, env *decimalEvalEnv) (any, error) {
	switch n.Operator {
	case "&&", "and":
		left, err := evalNode(n.Left, env)
		if err != nil {
			return nil, err
		}
		ok, err := truthy(left)
		if err != nil {
			return nil, err
		}
		if !ok {
			return false, nil
		}
		right, err := evalNode(n.Right, env)
		if err != nil {
			return nil, err
		}
		return truthy(right)
	case "||", "or":
		left, err := evalNode(n.Left, env)
		if err != nil {
			return nil, err
		}
		ok, err := truthy(left)
		if err != nil {
			return nil, err
		}
		if ok {
			return true, nil
		}
		right, err := evalNode(n.Right, env)
		if err != nil {
			return nil, err
		}
		return truthy(right)
	}

	left, err := evalNode(n.Left, env)
	if err != nil {
		return nil, err
	}
	right, err := evalNode(n.Right, env)
	if err != nil {
		return nil, err
	}

	switch n.Operator {
	case "==", "!=", "<", "<=", ">", ">=":
		return compareValues(n.Operator, left, right)
	case "+", "-", "*", "/", "%", "**", "^":
		return evalArithmetic(n.Operator, left, right)
	default:
		return nil, fmt.Errorf("expr run error: unsupported operator %q", n.Operator)
	}
}

func evalArithmetic(op string, left, right any) (any, error) {
	ld, lOK := valueAsDecimal(left)
	rd, rOK := valueAsDecimal(right)
	if !lOK || !rOK {
		return nil, fmt.Errorf("expr run error: operator %s requires numbers, got %T and %T", op, left, right)
	}
	switch op {
	case "+":
		return ld.Add(rd), nil
	case "-":
		return ld.Sub(rd), nil
	case "*":
		return ld.Mul(rd), nil
	case "/":
		if rd.IsZero() {
			return nil, errNonFiniteExpr
		}
		return ld.Div(rd), nil
	case "%":
		if rd.IsZero() {
			return nil, errNonFiniteExpr
		}
		return ld.Mod(rd), nil
	case "**", "^":
		return ld.Pow(rd), nil
	default:
		return nil, fmt.Errorf("expr run error: unsupported operator %q", op)
	}
}

func evalCall(n *ast.CallNode, env *decimalEvalEnv) (any, error) {
	ident, ok := n.Callee.(*ast.IdentifierNode)
	if !ok {
		return nil, fmt.Errorf("expr run error: unsupported callee %T", n.Callee)
	}
	args, err := evalArgs(n.Arguments, env)
	if err != nil {
		return nil, err
	}
	return evalFunction(ident.Value, args, env)
}

func evalBuiltin(n *ast.BuiltinNode, env *decimalEvalEnv) (any, error) {
	args, err := evalArgs(n.Arguments, env)
	if err != nil {
		return nil, err
	}
	return evalFunction(n.Name, args, env)
}

func evalArgs(nodes []ast.Node, env *decimalEvalEnv) ([]any, error) {
	out := make([]any, len(nodes))
	for i, node := range nodes {
		v, err := evalNode(node, env)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func evalFunction(name string, args []any, env *decimalEvalEnv) (any, error) {
	switch name {
	case "tier":
		if len(args) != 2 {
			return nil, fmt.Errorf("expr run error: tier expects 2 arguments")
		}
		label, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("expr run error: tier name must be a string, got %T", args[0])
		}
		cost, ok := valueAsDecimal(args[1])
		if !ok {
			return nil, fmt.Errorf("expr run error: tier value must be a number, got %T", args[1])
		}
		env.trace.MatchedTier = label
		return cost, nil
	case "fixed":
		if len(args) != 1 {
			return nil, fmt.Errorf("expr run error: fixed expects 1 argument")
		}
		amount, ok := valueAsDecimal(args[0])
		if !ok {
			return nil, fmt.Errorf("expr run error: fixed amount must be a number, got %T", args[0])
		}
		env.trace.BillingUnit = BillingUnitRequest
		view := amount.InexactFloat64()
		env.trace.FixedPrice = &view
		return amount.Mul(decimal.NewFromInt(1_000_000)), nil
	case requestRuleTraceFunction, requestRuleTraceIntFunction:
		if len(args) != 3 {
			return nil, fmt.Errorf("expr run error: internal trace expects 3 arguments")
		}
		index, ok := decimalAsInt(args[0])
		if !ok {
			return nil, fmt.Errorf("expr run error: internal trace index must be an integer, got %T", args[0])
		}
		matched, err := truthy(args[1])
		if err != nil {
			return nil, err
		}
		if matched && index >= 0 && index < len(env.trace.RequestRules) {
			env.trace.RequestRules[index].Matched = true
		}
		if !matched {
			return decimal.NewFromInt(1), nil
		}
		multiplier, ok := valueAsDecimal(args[2])
		if !ok {
			return nil, fmt.Errorf("expr run error: request multiplier must be a number, got %T", args[2])
		}
		return multiplier, nil
	case "header":
		if len(args) != 1 {
			return nil, fmt.Errorf("expr run error: header expects 1 argument")
		}
		key, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("expr run error: header key must be a string, got %T", args[0])
		}
		return env.headers[strings.ToLower(strings.TrimSpace(key))], nil
	case "param":
		if len(args) != 1 {
			return nil, fmt.Errorf("expr run error: param expects 1 argument")
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("expr run error: param path must be a string, got %T", args[0])
		}
		path = strings.TrimSpace(path)
		if path == "" || len(env.body) == 0 {
			return nil, nil
		}
		result := gjson.GetBytes(env.body, path)
		if !result.Exists() {
			return nil, nil
		}
		return result.Value(), nil
	case "u":
		if len(args) != 1 {
			return nil, fmt.Errorf("expr run error: u expects 1 argument")
		}
		key, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("expr run error: usage key must be a string, got %T", args[0])
		}
		if env.usage == nil {
			return nil, nil
		}
		return env.usage[strings.TrimSpace(key)], nil
	case "has":
		if len(args) != 2 {
			return nil, fmt.Errorf("expr run error: has expects 2 arguments")
		}
		substr, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("expr run error: has substring must be a string, got %T", args[1])
		}
		if args[0] == nil || substr == "" {
			return false, nil
		}
		return strings.Contains(fmt.Sprint(args[0]), substr), nil
	case "hour":
		tz, err := timezoneArg(args)
		if err != nil {
			return nil, err
		}
		return decimal.NewFromInt(int64(timeInZone(tz).Hour())), nil
	case "minute":
		tz, err := timezoneArg(args)
		if err != nil {
			return nil, err
		}
		return decimal.NewFromInt(int64(timeInZone(tz).Minute())), nil
	case "weekday":
		tz, err := timezoneArg(args)
		if err != nil {
			return nil, err
		}
		return decimal.NewFromInt(int64(timeInZone(tz).Weekday())), nil
	case "month":
		tz, err := timezoneArg(args)
		if err != nil {
			return nil, err
		}
		return decimal.NewFromInt(int64(timeInZone(tz).Month())), nil
	case "day":
		tz, err := timezoneArg(args)
		if err != nil {
			return nil, err
		}
		return decimal.NewFromInt(int64(timeInZone(tz).Day())), nil
	case "max", "min":
		if len(args) != 2 {
			return nil, fmt.Errorf("expr run error: %s expects 2 arguments", name)
		}
		left, lOK := valueAsDecimal(args[0])
		right, rOK := valueAsDecimal(args[1])
		if !lOK || !rOK {
			return nil, fmt.Errorf("expr run error: %s requires numbers, got %T and %T", name, args[0], args[1])
		}
		if name == "max" {
			return decimal.Max(left, right), nil
		}
		return decimal.Min(left, right), nil
	case "abs", "ceil", "floor":
		if len(args) != 1 {
			return nil, fmt.Errorf("expr run error: %s expects 1 argument", name)
		}
		d, ok := valueAsDecimal(args[0])
		if !ok {
			return nil, fmt.Errorf("expr run error: %s requires a number, got %T", name, args[0])
		}
		switch name {
		case "abs":
			return d.Abs(), nil
		case "ceil":
			return d.Ceil(), nil
		default:
			return d.Floor(), nil
		}
	default:
		return nil, fmt.Errorf("expr run error: unknown function %q", name)
	}
}

func timezoneArg(args []any) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("expr run error: timezone helper expects 1 argument")
	}
	tz, ok := args[0].(string)
	if !ok {
		return "", fmt.Errorf("expr run error: timezone must be a string, got %T", args[0])
	}
	return tz, nil
}

func decimalFromFloatNode(src string, node *ast.FloatNode) decimal.Decimal {
	if d, ok := decimalFromLocation(src, node.Location()); ok {
		return d
	}
	return decimal.NewFromFloat(node.Value)
}

func decimalFromLocation(src string, loc file.Location) (decimal.Decimal, bool) {
	if loc.From < 0 || loc.To < loc.From || loc.To > len(src) {
		return decimal.Zero, false
	}
	lit := strings.ReplaceAll(src[loc.From:loc.To], "_", "")
	if lit == "" {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(lit)
	if err != nil {
		return decimal.Zero, false
	}
	return d, true
}

func tokenParamsAsDecimals(params TokenParams, imageCount int) map[string]decimal.Decimal {
	return map[string]decimal.Decimal{
		"image_count": decimal.NewFromInt(int64(imageCount)),
		"p":           decimal.NewFromFloat(params.P),
		"c":           decimal.NewFromFloat(params.C),
		"len":         decimal.NewFromFloat(params.Len),
		"cr":          decimal.NewFromFloat(params.CR),
		"cc":          decimal.NewFromFloat(params.CC),
		"cc1h":        decimal.NewFromFloat(params.CC1h),
		"cc12h":       decimal.NewFromFloat(params.CC12h),
		"img":         decimal.NewFromFloat(params.Img),
		"img_cr":      decimal.NewFromFloat(params.ImgCR),
		"img_o":       decimal.NewFromFloat(params.ImgO),
		"ai":          decimal.NewFromFloat(params.AI),
		"ao":          decimal.NewFromFloat(params.AO),
	}
}

func valueAsDecimal(v any) (decimal.Decimal, bool) {
	switch x := v.(type) {
	case decimal.Decimal:
		return x, true
	case int:
		return decimal.NewFromInt(int64(x)), true
	case int8:
		return decimal.NewFromInt(int64(x)), true
	case int16:
		return decimal.NewFromInt(int64(x)), true
	case int32:
		return decimal.NewFromInt(int64(x)), true
	case int64:
		return decimal.NewFromInt(x), true
	case uint:
		return decimal.NewFromInt(int64(x)), true
	case uint8:
		return decimal.NewFromInt(int64(x)), true
	case uint16:
		return decimal.NewFromInt(int64(x)), true
	case uint32:
		return decimal.NewFromInt(int64(x)), true
	case uint64:
		if x > uint64(math.MaxInt64) {
			return decimal.Zero, false
		}
		return decimal.NewFromInt(int64(x)), true
	case float32:
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return decimal.Zero, false
		}
		return decimal.NewFromFloat(float64(x)), true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return decimal.Zero, false
		}
		return decimal.NewFromFloat(x), true
	default:
		return decimal.Zero, false
	}
}

func decimalAsInt(v any) (int, bool) {
	d, ok := valueAsDecimal(v)
	if !ok || !d.Equal(d.Truncate(0)) {
		return 0, false
	}
	return int(d.IntPart()), true
}

func decimalFiniteNonNegative(d decimal.Decimal) bool {
	return !d.IsNegative()
}

func truthy(v any) (bool, error) {
	switch x := v.(type) {
	case nil:
		return false, nil
	case bool:
		return x, nil
	case string:
		return x != "", nil
	default:
		if d, ok := valueAsDecimal(v); ok {
			return !d.IsZero(), nil
		}
		return false, fmt.Errorf("expr run error: cannot use %T as a condition", v)
	}
}

func compareValues(op string, left, right any) (bool, error) {
	if op == "==" || op == "!=" {
		eq, ok := exactEquality(left, right)
		if ok {
			if op == "!=" {
				return !eq, nil
			}
			return eq, nil
		}
	}
	ld, lOK := valueAsDecimal(left)
	rd, rOK := valueAsDecimal(right)
	if !lOK || !rOK {
		if op == "==" {
			return false, nil
		}
		if op == "!=" {
			return true, nil
		}
		return false, fmt.Errorf("expr run error: cannot compare %T and %T", left, right)
	}
	cmp := ld.Cmp(rd)
	switch op {
	case "==":
		return cmp == 0, nil
	case "!=":
		return cmp != 0, nil
	case "<":
		return cmp < 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case ">=":
		return cmp >= 0, nil
	default:
		return false, fmt.Errorf("expr run error: unsupported operator %q", op)
	}
}

func exactEquality(left, right any) (eq bool, handled bool) {
	if left == nil || right == nil {
		return left == nil && right == nil, true
	}
	if ls, lok := left.(string); lok {
		rs, rok := right.(string)
		return rok && ls == rs, true
	}
	if _, rok := right.(string); rok {
		return false, true
	}
	if lb, lok := left.(bool); lok {
		rb, rok := right.(bool)
		return rok && lb == rb, true
	}
	if _, rok := right.(bool); rok {
		return false, true
	}
	return false, false
}
