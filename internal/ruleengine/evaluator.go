package ruleengine

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// EvalContext provides field values for identifier resolution.
type EvalContext struct {
	Roots map[string]map[string]any
}

func NewEvalContext() *EvalContext {
	return &EvalContext{Roots: make(map[string]map[string]any)}
}

func (c *EvalContext) Set(root, field string, val any) {
	if c.Roots[root] == nil {
		c.Roots[root] = make(map[string]any)
	}
	c.Roots[root][field] = val
}

// Evidence captures what happened at each AST node during evaluation,
// used for trial calculations that must show why a rule matched.
type Evidence struct {
	NodeID   string
	NodeType string
	Source   string
	GotValue string
	Expected string
	Passed   bool
}

// Evaluator evaluates a checked AST against a context, collecting evidence.
type Evaluator struct {
	evidence []Evidence
}

func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// Eval evaluates the AST and returns the result. The result type depends
// on the root node type. Returns an error for type mismatches at runtime.
func (ev *Evaluator) Eval(node Node, ctx *EvalContext) (any, error) {
	ev.evidence = ev.evidence[:0]
	return ev.evalNode(node, ctx)
}

func (ev *Evaluator) evalNode(node Node, ctx *EvalContext) (any, error) {
	switch n := node.(type) {
	case *LiteralNode:
		return n.Value, nil
	case *IdentifierNode:
		return ev.evalIdentifier(n, ctx)
	case *BinaryOpNode:
		return ev.evalBinary(n, ctx)
	case *UnaryOpNode:
		return ev.evalUnary(n, ctx)
	case *CallNode:
		return ev.evalCall(n, ctx)
	}
	return nil, fmt.Errorf("cannot evaluate node type %T", node)
}

func (ev *Evaluator) evalIdentifier(n *IdentifierNode, ctx *EvalContext) (any, error) {
	if len(n.Path) < 2 {
		return nil, fmt.Errorf("identifier %s has no field", n.String())
	}
	root := n.Path[0]
	fields, ok := ctx.Roots[root]
	if !ok {
		return nil, fmt.Errorf("context root %q not found", root)
	}
	field := strings.Join(n.Path[1:], ".")
	val, ok := fields[field]
	if !ok {
		return nil, fmt.Errorf("field %q not found on root %q", field, root)
	}
	ev.recordEvidence(n.String(), "identifier", n.String(), fmt.Sprintf("%v", val), "", true)
	return val, nil
}

func (ev *Evaluator) evalBinary(n *BinaryOpNode, ctx *EvalContext) (any, error) {
	left, err := ev.evalNode(n.Left, ctx)
	if err != nil {
		return nil, err
	}
	if n.Op == "&&" {
		lb, ok := left.(bool)
		if !ok {
			return nil, fmt.Errorf("&& requires bool left operand")
		}
		if !lb {
			ev.recordEvidence(n.String(), "binary", n.String(), "false", "", true)
			return false, nil
		}
		right, err := ev.evalNode(n.Right, ctx)
		if err != nil {
			return nil, err
		}
		rb, ok := right.(bool)
		if !ok {
			return nil, fmt.Errorf("&& requires bool right operand")
		}
		result := rb
		ev.recordEvidence(n.String(), "binary", n.String(), strconv.FormatBool(result), "", result)
		return result, nil
	}
	if n.Op == "||" {
		lb, ok := left.(bool)
		if !ok {
			return nil, fmt.Errorf("|| requires bool left operand")
		}
		if lb {
			ev.recordEvidence(n.String(), "binary", n.String(), "true", "", true)
			return true, nil
		}
		right, err := ev.evalNode(n.Right, ctx)
		if err != nil {
			return nil, err
		}
		rb, ok := right.(bool)
		if !ok {
			return nil, fmt.Errorf("|| requires bool right operand")
		}
		result := rb
		ev.recordEvidence(n.String(), "binary", n.String(), strconv.FormatBool(result), "", result)
		return result, nil
	}
	right, err := ev.evalNode(n.Right, ctx)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case "==":
		result := equalValues(left, right)
		ev.recordEvidence(n.String(), "binary", n.String(), strconv.FormatBool(result), "", result)
		return result, nil
	case "!=":
		result := !equalValues(left, right)
		ev.recordEvidence(n.String(), "binary", n.String(), strconv.FormatBool(result), "", result)
		return result, nil
	case "<", ">", "<=", ">=":
		lf, rf, err := asNumbers(left, right, n.Op)
		if err != nil {
			return nil, err
		}
		var result bool
		switch n.Op {
		case "<":
			result = lf < rf
		case ">":
			result = lf > rf
		case "<=":
			result = lf <= rf
		case ">=":
			result = lf >= rf
		}
		ev.recordEvidence(n.String(), "binary", n.String(), fmt.Sprintf("%v %s %v = %v", lf, n.Op, rf, result), "", result)
		return result, nil
	case "+":
		if ls, ok := left.(string); ok {
			if rs, ok2 := right.(string); ok2 {
				return ls + rs, nil
			}
		}
		lf, rf, err := asNumbers(left, right, n.Op)
		if err != nil {
			return nil, err
		}
		return lf + rf, nil
	case "-":
		lf, rf, err := asNumbers(left, right, n.Op)
		if err != nil {
			return nil, err
		}
		return lf - rf, nil
	case "*":
		lf, rf, err := asNumbers(left, right, n.Op)
		if err != nil {
			return nil, err
		}
		return lf * rf, nil
	case "/":
		lf, rf, err := asNumbers(left, right, n.Op)
		if err != nil {
			return nil, err
		}
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return lf / rf, nil
	}
	return nil, fmt.Errorf("unknown binary op %s", n.Op)
}

func (ev *Evaluator) evalUnary(n *UnaryOpNode, ctx *EvalContext) (any, error) {
	val, err := ev.evalNode(n.Operand, ctx)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case "!":
		b, ok := val.(bool)
		if !ok {
			return nil, fmt.Errorf("! requires bool")
		}
		return !b, nil
	case "-":
		f, ok := toFloat(val)
		if !ok {
			return nil, fmt.Errorf("- requires number")
		}
		return -f, nil
	}
	return nil, fmt.Errorf("unknown unary op %s", n.Op)
}

func (ev *Evaluator) evalCall(n *CallNode, ctx *EvalContext) (any, error) {
	args := make([]any, len(n.Args))
	for i, arg := range n.Args {
		val, err := ev.evalNode(arg, ctx)
		if err != nil {
			return nil, err
		}
		args[i] = val
	}
	result, err := callBuiltin(n.FuncName, args)
	if err != nil {
		return nil, fmt.Errorf("function %s: %w", n.FuncName, err)
	}
	ev.recordEvidence(n.String(), "call", n.FuncName, fmt.Sprintf("%v", result), "", true)
	return result, nil
}

func (ev *Evaluator) recordEvidence(nodeID, nodeType, source, got, expected string, passed bool) {
	ev.evidence = append(ev.evidence, Evidence{
		NodeID:   nodeID,
		NodeType: nodeType,
		Source:   source,
		GotValue: got,
		Expected: expected,
		Passed:   passed,
	})
}

// Evidence returns the collected evaluation evidence.
func (ev *Evaluator) Evidence() []Evidence {
	return ev.evidence
}

func equalValues(a, b any) bool {
	if af, ok := toFloat(a); ok {
		if bf, ok2 := toFloat(b); ok2 {
			return af == bf
		}
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func asNumbers(a, b any, op string) (float64, float64, error) {
	af, ok := toFloat(a)
	if !ok {
		return 0, 0, fmt.Errorf("%s requires numbers, got %T", op, a)
	}
	bf, ok := toFloat(b)
	if !ok {
		return 0, 0, fmt.Errorf("%s requires numbers, got %T", op, b)
	}
	return af, bf, nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case bool:
		return 0, false
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// IsTruthy returns the boolean result of an evaluation.
func IsTruthy(val any) bool {
	if b, ok := val.(bool); ok {
		return b
	}
	if f, ok := toFloat(val); ok {
		return f != 0
	}
	return val != nil
}

var _ = math.Abs
