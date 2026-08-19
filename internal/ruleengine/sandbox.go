package ruleengine

import (
	"encoding/json"
	"fmt"
)

// Engine is the top-level rule expression engine. It compiles, checks,
// and evaluates rule expressions. It also provides a trial sandbox
// that shows matched evidence before a rule goes live.
type Engine struct {
	schema Schema
}

func NewEngine() *Engine {
	return &Engine{schema: NewDefaultSchema()}
}

// Compile lexes, parses, and type-checks a rule expression. Returns
// a checked AST ready for evaluation.
func (e *Engine) Compile(expr string) (Node, error) {
	lexer := NewLexer(expr)
	tokens, err := lexer.Lex()
	if err != nil {
		return nil, fmt.Errorf("lex: %w", err)
	}
	parser := NewParser(tokens)
	ast, err := parser.Parse()
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	checker := NewChecker(e.schema)
	checked, err := checker.Check(ast)
	if err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}
	return checked, nil
}

// Evaluate compiles and evaluates a rule expression against the given context.
func (e *Engine) Evaluate(expr string, ctx *EvalContext) (any, []Evidence, error) {
	node, err := e.Compile(expr)
	if err != nil {
		return nil, nil, err
	}
	ev := NewEvaluator()
	result, err := ev.Eval(node, ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("eval: %w", err)
	}
	return result, ev.Evidence(), nil
}

// TrialResult holds the outcome of a sandbox trial calculation.
type TrialResult struct {
	Matched  bool       `json:"matched"`
	Value    any        `json:"value"`
	Evidence []Evidence `json:"evidence"`
	Error    string     `json:"error,omitempty"`
}

// Trial runs a rule expression against a trial context (from JSON),
// collecting evidence so the auditor can verify why it matched or not.
// This is used before a rule is activated.
func (e *Engine) Trial(expr string, contextJSON string) (TrialResult, error) {
	ctx, err := parseContextJSON(contextJSON)
	if err != nil {
		return TrialResult{}, fmt.Errorf("parse context: %w", err)
	}
	node, err := e.Compile(expr)
	if err != nil {
		return TrialResult{Error: err.Error()}, err
	}
	ev := NewEvaluator()
	result, err := ev.Eval(node, ctx)
	if err != nil {
		return TrialResult{Error: err.Error()}, nil
	}
	return TrialResult{
		Matched:  IsTruthy(result),
		Value:    result,
		Evidence: ev.Evidence(),
	}, nil
}

// parseContextJSON deserializes a JSON object into an EvalContext.
// Expected format: {"submission": {"risk_level": 3, ...}, "standard": {...}, ...}
func parseContextJSON(data string) (*EvalContext, error) {
	if data == "" {
		return NewEvalContext(), nil
	}
	var raw map[string]map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, err
	}
	ctx := NewEvalContext()
	for root, fields := range raw {
		for field, val := range fields {
			ctx.Set(root, field, val)
		}
	}
	return ctx, nil
}

// FormatError formats a rule engine error with position context.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
