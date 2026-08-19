package ruleengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLexer_Tokens(t *testing.T) {
	tests := []struct {
		input     string
		wantTypes []TokenType
	}{
		{`submission.riskLevel >= 2`, []TokenType{TokenIdent, TokenDot, TokenIdent, TokenOp, TokenNumber, TokenEOF}},
		{`"hello" == "world"`, []TokenType{TokenString, TokenOp, TokenString, TokenEOF}},
		{`true && false`, []TokenType{TokenBool, TokenOp, TokenBool, TokenEOF}},
		{`contains(x, "y")`, []TokenType{TokenIdent, TokenLParen, TokenIdent, TokenComma, TokenString, TokenRParen, TokenEOF}},
	}
	for _, tt := range tests {
		l := NewLexer(tt.input)
		tokens, err := l.Lex()
		require.NoError(t, err, "input: %s", tt.input)
		assert.Equal(t, len(tt.wantTypes), len(tokens), "token count for: %s", tt.input)
	}
}

func TestLexer_UnterminatedString(t *testing.T) {
	l := NewLexer(`"unterminated`)
	_, err := l.Lex()
	assert.Error(t, err)
}

func TestLexer_Comments(t *testing.T) {
	l := NewLexer(`// line comment\nx == 1 /* block */`)
	_, err := l.Lex()
	assert.NoError(t, err)
}

func TestParser_BasicExpression(t *testing.T) {
	lexer := NewLexer(`submission.risk_level >= 2`)
	tokens, err := lexer.Lex()
	require.NoError(t, err)

	parser := NewParser(tokens)
	node, err := parser.Parse()
	require.NoError(t, err)
	assert.NotNil(t, node)

	binOp, ok := node.(*BinaryOpNode)
	require.True(t, ok)
	assert.Equal(t, ">=", binOp.Op)
}

func TestParser_LogicalAnd(t *testing.T) {
	lexer := NewLexer(`submission.category == "exoskeleton" && submission.risk_level >= 2`)
	tokens, err := lexer.Lex()
	require.NoError(t, err)

	parser := NewParser(tokens)
	node, err := parser.Parse()
	require.NoError(t, err)

	andNode, ok := node.(*BinaryOpNode)
	require.True(t, ok)
	assert.Equal(t, "&&", andNode.Op)
}

func TestParser_FunctionCall(t *testing.T) {
	lexer := NewLexer(`contains(submission.model_name, "rehab")`)
	tokens, err := lexer.Lex()
	require.NoError(t, err)

	parser := NewParser(tokens)
	node, err := parser.Parse()
	require.NoError(t, err)

	call, ok := node.(*CallNode)
	require.True(t, ok)
	assert.Equal(t, "contains", call.FuncName)
	assert.Len(t, call.Args, 2)
}

func TestParser_UnaryNot(t *testing.T) {
	lexer := NewLexer(`!contains(submission.model_name, "test")`)
	tokens, err := lexer.Lex()
	require.NoError(t, err)

	parser := NewParser(tokens)
	node, err := parser.Parse()
	require.NoError(t, err)

	unary, ok := node.(*UnaryOpNode)
	require.True(t, ok)
	assert.Equal(t, "!", unary.Op)
}

func TestParser_ParenthesizedExpression(t *testing.T) {
	lexer := NewLexer(`(submission.risk_level + 1) * 2`)
	tokens, err := lexer.Lex()
	require.NoError(t, err)

	parser := NewParser(tokens)
	_, err = parser.Parse()
	require.NoError(t, err)
}

func TestParser_InvalidExpression(t *testing.T) {
	lexer := NewLexer(`submission. >= 2`)
	tokens, err := lexer.Lex()
	require.NoError(t, err)

	parser := NewParser(tokens)
	_, err = parser.Parse()
	assert.Error(t, err)
}

func TestChecker_ValidRule(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "risk_level", 3.0)
	ctx.Set("submission", "category", "exoskeleton")

	result, evidence, err := engine.Evaluate(`submission.risk_level >= 2 && submission.category == "exoskeleton"`, ctx)
	require.NoError(t, err)
	assert.True(t, IsTruthy(result))
	assert.NotEmpty(t, evidence)
}

func TestChecker_UnknownField(t *testing.T) {
	engine := NewEngine()
	_, err := engine.Compile(`submission.nonexistent_field == 1`)
	assert.Error(t, err)
}

func TestChecker_TypeMismatch(t *testing.T) {
	engine := NewEngine()
	_, err := engine.Compile(`submission.risk_level && submission.category`)
	assert.Error(t, err)
}

func TestEvaluator_NumberComparison(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "risk_level", 3.0)

	result, _, err := engine.Evaluate(`submission.risk_level > 2`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestEvaluator_StringEquality(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "category", "exoskeleton")

	result, _, err := engine.Evaluate(`submission.category == "exoskeleton"`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestEvaluator_Contains(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "model_name", "RehabExoskeletonV2")

	result, _, err := engine.Evaluate(`contains(submission.model_name, "Rehab")`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestEvaluator_StartsWithAndEndsWith(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "model_no", "EXO-2024-001")

	result, _, err := engine.Evaluate(`startsWith(submission.model_no, "EXO")`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))

	result, _, err = engine.Evaluate(`endsWith(submission.model_no, "001")`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestEvaluator_LogicalOr(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "risk_level", 1.0)

	result, _, err := engine.Evaluate(`submission.risk_level >= 3 || submission.risk_level <= 1`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestEvaluator_DivisionByZero(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "risk_level", 5.0)

	_, _, err := engine.Evaluate(`submission.risk_level / 0`, ctx)
	assert.Error(t, err)
}

func TestEvaluator_NestedExpression(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "risk_level", 3.0)
	ctx.Set("submission", "category", "exoskeleton")
	ctx.Set("standard", "code", "GB-9706")

	result, _, err := engine.Evaluate(
		`(submission.risk_level >= 2 && submission.category == "exoskeleton") || standard.code == "GB-9706"`,
		ctx,
	)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestEvaluator_Arithmetic(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "risk_level", 3.0)

	result, _, err := engine.Evaluate(`submission.risk_level + 2 == 5`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))

	result, _, err = engine.Evaluate(`submission.risk_level * 2 == 6`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestTrial_Sandbox(t *testing.T) {
	engine := NewEngine()
	contextJSON := `{
		"submission": {"risk_level": 3, "category": "exoskeleton", "model_name": "RehabV1"}
	}`
	result, err := engine.Trial(`submission.risk_level >= 2 && submission.category == "exoskeleton"`, contextJSON)
	require.NoError(t, err)
	assert.True(t, result.Matched)
	assert.NotEmpty(t, result.Evidence)
}

func TestTrial_SandboxNoMatch(t *testing.T) {
	engine := NewEngine()
	contextJSON := `{
		"submission": {"risk_level": 1, "category": "prosthesis"}
	}`
	result, err := engine.Trial(`submission.risk_level >= 2 && submission.category == "exoskeleton"`, contextJSON)
	require.NoError(t, err)
	assert.False(t, result.Matched)
}

func TestTrial_InvalidExpression(t *testing.T) {
	engine := NewEngine()
	contextJSON := `{"submission": {"risk_level": 3}}`
	result, err := engine.Trial(`submission.risk_level >>>`, contextJSON)
	assert.Error(t, err)
	_ = result
}

func TestTrial_BadContextJSON(t *testing.T) {
	engine := NewEngine()
	_, err := engine.Trial(`submission.risk_level >= 2`, `{invalid json}`)
	assert.Error(t, err)
}

func TestBuiltins_Len(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "model_name", "Rehab")

	result, _, err := engine.Evaluate(`len(submission.model_name) == 5`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestBuiltins_Abs(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "risk_level", -3.0)

	result, _, err := engine.Evaluate(`abs(submission.risk_level) == 3`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestBuiltins_UpperLower(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "category", "Exoskeleton")

	result, _, err := engine.Evaluate(`upper(submission.category) == "EXOSKELETON"`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))

	result, _, err = engine.Evaluate(`lower(submission.category) == "exoskeleton"`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestBuiltins_Num(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "batch_no", "3")

	result, _, err := engine.Evaluate(`num(submission.batch_no) == 3`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}

func TestBuiltins_In(t *testing.T) {
	engine := NewEngine()
	ctx := NewEvalContext()
	ctx.Set("submission", "category", "exoskeleton")
	ctx.Set("submission", "standard_codes", []string{"GB-9706", "YY-0505"})

	result, _, err := engine.Evaluate(`in("GB-9706", submission.standard_codes)`, ctx)
	require.NoError(t, err)
	assert.True(t, result.(bool))
}
