package ruleengine

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// callBuiltin dispatches to the appropriate built-in function.
func callBuiltin(name string, args []any) (any, error) {
	switch name {
	case "contains":
		return builtinContains(args)
	case "startsWith":
		return builtinStartsWith(args)
	case "endsWith":
		return builtinEndsWith(args)
	case "upper":
		return strings.ToUpper(toString(args[0])), nil
	case "lower":
		return strings.ToLower(toString(args[0])), nil
	case "len":
		return builtinLen(args)
	case "in":
		return builtinIn(args)
	case "abs":
		f, ok := toFloat(args[0])
		if !ok {
			return nil, fmt.Errorf("abs requires a number")
		}
		return math.Abs(f), nil
	case "round":
		f, ok := toFloat(args[0])
		if !ok {
			return nil, fmt.Errorf("round requires a number")
		}
		return math.Round(f), nil
	case "num":
		s := toString(args[0])
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("num: %w", err)
		}
		return f, nil
	case "str":
		return toString(args[0]), nil
	default:
		return nil, fmt.Errorf("unknown function %q", name)
	}
}

func builtinContains(args []any) (bool, error) {
	s := toString(args[0])
	sub := toString(args[1])
	return strings.Contains(s, sub), nil
}

func builtinStartsWith(args []any) (bool, error) {
	s := toString(args[0])
	prefix := toString(args[1])
	return strings.HasPrefix(s, prefix), nil
}

func builtinEndsWith(args []any) (bool, error) {
	s := toString(args[0])
	suffix := toString(args[1])
	return strings.HasSuffix(s, suffix), nil
}

func builtinLen(args []any) (float64, error) {
	switch v := args[0].(type) {
	case string:
		return float64(len(v)), nil
	case []string:
		return float64(len(v)), nil
	case []any:
		return float64(len(v)), nil
	}
	s := toString(args[0])
	return float64(len(s)), nil
}

func builtinIn(args []any) (bool, error) {
	item := toString(args[0])
	switch list := args[1].(type) {
	case []string:
		for _, s := range list {
			if s == item {
				return true, nil
			}
		}
		return false, nil
	case []any:
		for _, v := range list {
			if toString(v) == item {
				return true, nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("in: second argument must be a list")
}

func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case bool:
		return strconv.FormatBool(s)
	case float64:
		if s == math.Trunc(s) {
			return strconv.FormatInt(int64(s), 10)
		}
		return strconv.FormatFloat(s, 'f', -1, 64)
	case int:
		return strconv.Itoa(s)
	case int64:
		return strconv.FormatInt(s, 10)
	case nil:
		return ""
	}
	return fmt.Sprintf("%v", v)
}
