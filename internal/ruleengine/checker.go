package ruleengine

import (
	"fmt"
	"strings"
)

// FieldDef describes one field in the evaluation context schema.
type FieldDef struct {
	Type ValueType
}

// Schema defines the structure of the evaluation context that rules
// can reference. The checker validates that all identifier paths in a
// rule expression resolve to fields defined here.
type Schema struct {
	Roots map[string]map[string]FieldDef
}

// NewSchema returns a schema with the standard evaluation context roots.
func NewDefaultSchema() Schema {
	return Schema{
		Roots: map[string]map[string]FieldDef{
			"submission": {
				"model_no":       {Type: TypeString},
				"model_name":     {Type: TypeString},
				"category":       {Type: TypeString},
				"risk_level":     {Type: TypeNumber},
				"enterprise_id":  {Type: TypeString},
				"batch_no":       {Type: TypeString},
				"standard_codes": {Type: TypeList},
				"status":         {Type: TypeString},
			},
			"standard": {
				"code":     {Type: TypeString},
				"name":     {Type: TypeString},
				"category": {Type: TypeString},
			},
			"agency": {
				"id":                   {Type: TypeString},
				"code":                 {Type: TypeString},
				"name":                 {Type: TypeString},
				"accredited_standards": {Type: TypeList},
			},
			"model": {
				"no":         {Type: TypeString},
				"name":       {Type: TypeString},
				"category":   {Type: TypeString},
				"risk_level": {Type: TypeNumber},
			},
		},
	}
}

// Checker validates an AST against a schema: type checking and field
// reference validation. Returns a typed AST with inferred types.
type Checker struct {
	schema Schema
	errors []string
}

func NewChecker(schema Schema) *Checker {
	return &Checker{schema: schema}
}

func (c *Checker) Check(node Node) (Node, error) {
	result := c.check(node)
	if len(c.errors) > 0 {
		return nil, fmt.Errorf("type check failed: %s", strings.Join(c.errors, "; "))
	}
	return result, nil
}

func (c *Checker) check(node Node) Node {
	switch n := node.(type) {
	case *LiteralNode:
		return n
	case *IdentifierNode:
		return c.checkIdentifier(n)
	case *BinaryOpNode:
		return c.checkBinaryOp(n)
	case *UnaryOpNode:
		return c.checkUnaryOp(n)
	case *CallNode:
		return c.checkCall(n)
	}
	c.addError(node, "unknown node type")
	return node
}

func (c *Checker) checkIdentifier(n *IdentifierNode) Node {
	if len(n.Path) < 2 {
		c.addError(n, "identifier must have at least root.field, got %s", n.String())
		return n
	}
	root := n.Path[0]
	fields, ok := c.schema.Roots[root]
	if !ok {
		c.addError(n, "unknown context root %q in %s", root, n.String())
		return n
	}
	field := strings.Join(n.Path[1:], ".")
	def, ok := fields[field]
	if !ok {
		c.addError(n, "unknown field %q on root %q", field, root)
		return n
	}
	n.VType = def.Type
	return n
}

func (c *Checker) checkBinaryOp(n *BinaryOpNode) Node {
	n.Left = c.check(n.Left)
	n.Right = c.check(n.Right)
	lt := n.Left.Type()
	rt := n.Right.Type()
	switch n.Op {
	case "==", "!=":
		if lt != rt && lt != TypeUnknown && rt != TypeUnknown {
			c.addError(n, "type mismatch in %s: %s vs %s", n.Op, lt, rt)
		}
	case "<", ">", "<=", ">=":
		if lt != TypeNumber && lt != TypeString {
			c.addError(n, "comparison requires number or string, got %s", lt)
		}
		if rt != TypeNumber && rt != TypeString {
			c.addError(n, "comparison requires number or string, got %s", rt)
		}
	case "&&", "||":
		if lt != TypeBool {
			c.addError(n, "logical op requires bool, got %s", lt)
		}
		if rt != TypeBool {
			c.addError(n, "logical op requires bool, got %s", rt)
		}
	case "+":
		if lt == TypeString && rt == TypeString {
			return n
		}
		if lt != TypeNumber || rt != TypeNumber {
			c.addError(n, "+ requires numbers or strings, got %s and %s", lt, rt)
		}
	case "-", "*", "/":
		if lt != TypeNumber || rt != TypeNumber {
			c.addError(n, "%s requires numbers, got %s and %s", n.Op, lt, rt)
		}
	}
	return n
}

func (c *Checker) checkUnaryOp(n *UnaryOpNode) Node {
	n.Operand = c.check(n.Operand)
	ot := n.Operand.Type()
	switch n.Op {
	case "!":
		if ot != TypeBool {
			c.addError(n, "! requires bool, got %s", ot)
		}
	case "-":
		if ot != TypeNumber {
			c.addError(n, "- requires number, got %s", ot)
		}
	}
	return n
}

func (c *Checker) checkCall(n *CallNode) Node {
	for i := range n.Args {
		n.Args[i] = c.check(n.Args[i])
	}
	sig, ok := builtinSignatures[n.FuncName]
	if !ok {
		c.addError(n, "unknown function %q", n.FuncName)
		return n
	}
	if len(n.Args) != sig.Arity {
		c.addError(n, "function %q expects %d args, got %d", n.FuncName, sig.Arity, len(n.Args))
		return n
	}
	for i, arg := range n.Args {
		if sig.ParamTypes[i] != TypeUnknown && arg.Type() != sig.ParamTypes[i] && arg.Type() != TypeUnknown {
			c.addError(n, "function %q arg %d expects %s, got %s", n.FuncName, i+1, sig.ParamTypes[i], arg.Type())
		}
	}
	n.VType = sig.ReturnType
	return n
}

func (c *Checker) addError(node Node, format string, args ...any) {
	line, col := node.Pos()
	c.errors = append(c.errors, fmt.Sprintf("line %d col %d: %s", line, col, fmt.Sprintf(format, args...)))
}

// FuncSignature describes a built-in function's parameter and return types.
type FuncSignature struct {
	Arity      int
	ParamTypes []ValueType
	ReturnType ValueType
}

var builtinSignatures = map[string]FuncSignature{
	"contains":   {Arity: 2, ParamTypes: []ValueType{TypeString, TypeString}, ReturnType: TypeBool},
	"startsWith": {Arity: 2, ParamTypes: []ValueType{TypeString, TypeString}, ReturnType: TypeBool},
	"endsWith":   {Arity: 2, ParamTypes: []ValueType{TypeString, TypeString}, ReturnType: TypeBool},
	"upper":      {Arity: 1, ParamTypes: []ValueType{TypeString}, ReturnType: TypeString},
	"lower":      {Arity: 1, ParamTypes: []ValueType{TypeString}, ReturnType: TypeString},
	"len":        {Arity: 1, ParamTypes: []ValueType{TypeUnknown}, ReturnType: TypeNumber},
	"in":         {Arity: 2, ParamTypes: []ValueType{TypeString, TypeList}, ReturnType: TypeBool},
	"abs":        {Arity: 1, ParamTypes: []ValueType{TypeNumber}, ReturnType: TypeNumber},
	"round":      {Arity: 1, ParamTypes: []ValueType{TypeNumber}, ReturnType: TypeNumber},
	"num":        {Arity: 1, ParamTypes: []ValueType{TypeString}, ReturnType: TypeNumber},
	"str":        {Arity: 1, ParamTypes: []ValueType{TypeNumber}, ReturnType: TypeString},
}
