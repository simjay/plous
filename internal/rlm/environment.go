package rlm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

const maxItems = 4096

func validateSyntax(file *syntax.File) error {
	var err error
	var stack []syntax.Node
	var comparisons []*syntax.BinaryExpr
	syntax.Walk(file, func(node syntax.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if err != nil {
			return false
		}
		var parent syntax.Node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		stack = append(stack, node)
		switch n := node.(type) {
		case *syntax.Ident:
			if n.Name == "__scalar" {
				err = errors.New("reserved interpreter name")
			}
		case *syntax.TupleExpr:
			binding := false
			switch p := parent.(type) {
			case *syntax.AssignStmt:
				binding = p.LHS == n
			case *syntax.ForStmt:
				binding = p.Vars == n
			case *syntax.ForClause:
				binding = p.Vars == n
			}
			if !binding {
				err = errors.New("tuple values are unavailable; use lists")
			}
		case *syntax.LoadStmt:
			err = errors.New("module loading is unavailable")
		case *syntax.SliceExpr:
			err = errors.New("slice copying is unavailable; filter with a comprehension")
		case *syntax.BinaryExpr:
			switch n.Op {
			case syntax.EQL, syntax.NEQ, syntax.LT, syntax.LE, syntax.GT, syntax.GE, syntax.IN, syntax.NOT_IN:
				comparisons = append(comparisons, n)
			case syntax.AND, syntax.OR, syntax.EQ:
			default:
				err = errors.New("arithmetic and concatenation operators are unavailable; use sum or join")
			}
		case *syntax.UnaryExpr:
			if n.Op != syntax.NOT {
				literal, ok := n.X.(*syntax.Literal)
				if n.Op != syntax.MINUS || !ok || literal.Token != syntax.INT {
					err = errors.New("only literal negative integers are supported")
				}
			}
		case *syntax.AssignStmt:
			if n.Op != syntax.EQ {
				err = errors.New("augmented assignment is unavailable")
			}
		case *syntax.Literal:
			if n.Token == syntax.INT && len(n.Raw) > 20 {
				err = errors.New("integer literal exceeds limit")
			}
		case *syntax.DotExpr:
			switch n.Name.Name {
			case "get", "append", "pop", "clear", "setdefault", "startswith", "endswith", "find", "rfind", "count", "isdigit", "isalpha", "isalnum", "islower", "isupper", "isspace":
			default:
				err = fmt.Errorf("attribute %q is unavailable", n.Name.Name)
			}
		}
		if err != nil {
			stack = stack[:len(stack)-1]
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	// Native collection equality can traverse shared trees exponentially within
	// one interpreter step. Comparisons are restricted to scalar operands.
	for _, comparison := range comparisons {
		wrap := func(value syntax.Expr) syntax.Expr {
			return &syntax.CallExpr{Fn: &syntax.Ident{Name: "__scalar", NamePos: syntax.Start(value)}, Args: []syntax.Expr{value}}
		}
		comparison.X = wrap(comparison.X)
		if comparison.Op != syntax.IN && comparison.Op != syntax.NOT_IN {
			comparison.Y = wrap(comparison.Y)
		}
	}
	return err
}

func (f *frame) environment() starlark.StringDict {
	env := starlark.StringDict{}
	for name, value := range starlark.Universe {
		if _, ok := value.(*starlark.Builtin); !ok {
			env[name] = value
			continue
		}
		env[name] = starlark.NewBuiltin(name, func(_ *starlark.Thread, b *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return nil, fmt.Errorf("builtin %s is unavailable", b.Name())
		})
	}
	for _, name := range []string{"len", "bool", "type", "any", "all", "range", "list", "dict", "reversed"} {
		original := starlark.Universe[name]
		env[name] = starlark.NewBuiltin(name, func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if b.Name() == "len" || b.Name() == "bool" || b.Name() == "type" {
				return starlark.Call(t, original, args, kwargs)
			}
			cost := 64
			for _, arg := range args {
				length := starlark.Len(arg)
				if length > maxItems {
					return nil, errors.New("collection argument exceeds limit")
				}
				if length > 0 {
					cost += length * 64
				}
			}
			if b.Name() == "range" {
				for _, arg := range args {
					value, ok := arg.(starlark.Int)
					if !ok {
						return nil, errors.New("range requires integers")
					}
					n, valid := value.Int64()
					if !valid || n < -maxItems || n > maxItems {
						return nil, errors.New("range arguments exceed limit")
					}
				}
			}
			if err := f.op.allocate(cost); err != nil {
				f.fatal = err
				return nil, err
			}
			return starlark.Call(t, original, args, kwargs)
		})
	}
	env["__scalar"] = starlark.NewBuiltin("__scalar", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		switch args[0].(type) {
		case starlark.NoneType, starlark.String, starlark.Int, starlark.Float, starlark.Bool:
			return args[0], nil
		default:
			return nil, errors.New("comparison requires a scalar value")
		}
	})
	env["enumerate"] = starlark.NewBuiltin("enumerate", f.enumerate)
	env["read"] = f.tool("read", f.op.runtime.Read)
	env["call"] = f.tool("call", f.op.runtime.Call)
	env["ask"] = starlark.NewBuiltin("ask", f.ask)
	env["finish"] = starlark.NewBuiltin("finish", f.finish)
	env["text"] = starlark.NewBuiltin("text", f.text)
	env["print"] = starlark.NewBuiltin("print", f.print)
	env["lines"] = starlark.NewBuiltin("lines", f.lines)
	env["join"] = starlark.NewBuiltin("join", f.join)
	env["sum"] = starlark.NewBuiltin("sum", f.sum)
	return env
}

func (f *frame) enumerate(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var values starlark.Iterable
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "values", &values); err != nil {
		return nil, err
	}
	iterator := values.Iterate()
	defer iterator.Done()
	var item starlark.Value
	var result []starlark.Value
	for iterator.Next(&item) {
		if len(result) >= maxItems {
			return nil, errors.New("enumeration exceeds limit")
		}
		if err := f.op.allocate(128); err != nil {
			f.fatal = err
			return nil, err
		}
		result = append(result, starlark.NewList([]starlark.Value{starlark.MakeInt(len(result)), item}))
	}
	return starlark.NewList(result), nil
}

func (f *frame) tool(name string, callback func(context.Context, string) (string, error)) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := f.ctx.Err(); err != nil {
			f.fatal = err
			return nil, err
		}
		if f.result != nil {
			return nil, errors.New("investigation already finished")
		}
		var input *starlark.Dict
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "request", &input); err != nil {
			return nil, err
		}
		encoded, err := encodeValue(input, f.op.limits.PromptBytes/2)
		if err != nil {
			return nil, err
		}
		if f.op.toolCalls >= f.op.limits.ToolCalls {
			f.fatal = fmt.Errorf("%w: tool calls", ErrBudget)
			return nil, f.fatal
		}
		f.op.toolCalls++
		id := f.op.nextID()
		var output string
		if callback == nil {
			err = errors.New("tool unavailable in this scope")
		} else {
			output, err = callback(context.WithValue(f.ctx, callParentKey{}, f.id), encoded)
		}
		if f.ctx.Err() != nil {
			output, err = "", f.ctx.Err()
		}
		if len(output) > f.op.limits.ResultBytes {
			err = fmt.Errorf("%w: tool result size", ErrBudget)
			output = ""
			f.fatal = err
		}
		event := Event{ID: id, ParentID: f.id, Kind: name, Input: encoded, Output: output, Depth: f.depth}
		if err != nil {
			event.Error = bounded(err.Error(), f.op.limits.OutputBytes)
		}
		if traceErr := f.op.trace(f.ctx, event); traceErr != nil {
			f.fatal = traceErr
			return nil, traceErr
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrBudget) {
				f.fatal = err
				return nil, err
			}
			denied := starlark.NewDict(1)
			_ = denied.SetKey(starlark.String("error"), starlark.String(event.Error))
			return denied, nil
		}
		if err := f.op.allocate(len(output) * 8); err != nil {
			f.fatal = err
			return nil, err
		}
		return decodeValue(output)
	})
}

func (f *frame) ask(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var question, evidence string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "question", &question, "evidence?", &evidence); err != nil {
		return nil, err
	}
	if f.result != nil {
		return nil, errors.New("investigation already finished")
	}
	f.syncSteps()
	result, err := f.op.investigate(f.ctx, question, evidence, f.id, f.depth+1)
	if err != nil {
		f.fatal = err
		return nil, err
	}
	if f.op.steps >= f.op.limits.Steps {
		f.fatal = fmt.Errorf("%w: script steps", ErrBudget)
		return nil, f.fatal
	}
	f.thread.SetMaxExecutionSteps(f.thread.ExecutionSteps() + f.op.limits.Steps - f.op.steps)
	encoded, _ := json.Marshal(result)
	return decodeValue(string(encoded))
}

func (f *frame) finish(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var findings string
	var evidence starlark.Value = starlark.NewList(nil)
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "findings", &findings, "evidence?", &evidence); err != nil {
		return nil, err
	}
	if len(findings) > f.op.limits.OutputBytes {
		return nil, errors.New("findings exceed output limit")
	}
	refs, err := stringItems(evidence, 128)
	if err != nil {
		return nil, errors.New("finish evidence must be a flat list of at most 128 strings; pass child[\"evidence\"] directly or append its individual references")
	}
	resultSize := len(findings)
	for _, ref := range refs {
		if len(ref) > 256 {
			return nil, errors.New("evidence reference exceeds limit")
		}
		resultSize += len(ref) + 3
	}
	if resultSize > f.op.limits.OutputBytes {
		return nil, errors.New("findings and evidence exceed output limit")
	}
	f.result = &Result{Findings: findings, Evidence: refs}
	return starlark.None, nil
}

func (f *frame) text(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "value", &value); err != nil {
		return nil, err
	}
	encoded, err := encodeValue(value, f.op.limits.OutputBytes)
	if err != nil {
		return nil, err
	}
	if err := f.op.allocate(len(encoded)); err != nil {
		f.fatal = err
		return nil, err
	}
	return starlark.String(encoded), nil
}

func (f *frame) print(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) != 0 {
		return nil, errors.New("print accepts positional values only")
	}
	for _, value := range args {
		encoded, err := encodeValue(value, f.op.limits.OutputBytes-f.output.Len())
		if err != nil {
			return nil, err
		}
		if len(encoded)+1 > f.op.limits.OutputBytes-f.output.Len() {
			return nil, errors.New("printed output exceeds limit")
		}
		f.output.WriteString(encoded)
		f.output.WriteByte('\n')
	}
	return starlark.None, nil
}

func (f *frame) lines(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "text", &value); err != nil {
		return nil, err
	}
	n := strings.Count(value, "\n") + 1
	if n > maxItems {
		return nil, errors.New("line count exceeds limit")
	}
	if err := f.op.allocate(len(value) + n*32); err != nil {
		f.fatal = err
		return nil, err
	}
	values := make([]starlark.Value, 0, n)
	for _, line := range strings.Split(value, "\n") {
		values = append(values, starlark.String(line))
	}
	return starlark.NewList(values), nil
}

func (f *frame) join(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var parts starlark.Value
	var separator string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "parts", &parts, "sep?", &separator); err != nil {
		return nil, err
	}
	values, err := stringItems(parts, maxItems)
	if err != nil {
		return nil, err
	}
	if err := f.op.allocate(len(values) * 32); err != nil {
		f.fatal = err
		return nil, err
	}
	n := 0
	for _, s := range values {
		if len(s)+len(separator) > f.op.limits.OutputBytes-n {
			return nil, errors.New("joined text exceeds limit")
		}
		n += len(s) + len(separator)
	}
	if err := f.op.allocate(n); err != nil {
		f.fatal = err
		return nil, err
	}
	return starlark.String(strings.Join(values, separator)), nil
}

func (f *frame) sum(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var values starlark.Iterable
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "numbers", &values); err != nil {
		return nil, err
	}
	iterator := values.Iterate()
	defer iterator.Done()
	var value starlark.Value
	var total int64
	for count := 0; iterator.Next(&value); count++ {
		if count >= maxItems {
			return nil, errors.New("sum input exceeds limit")
		}
		n, ok := value.(starlark.Int)
		if !ok {
			return nil, errors.New("sum accepts integers")
		}
		v, ok := n.Int64()
		if !ok || v > 0 && total > math.MaxInt64-v || v < 0 && total < math.MinInt64-v {
			return nil, errors.New("sum exceeds integer range")
		}
		total += v
	}
	return starlark.MakeInt64(total), nil
}

func stringItems(value starlark.Value, max int) ([]string, error) {
	iterable, ok := value.(starlark.Iterable)
	if !ok {
		return nil, errors.New("expected a list of strings")
	}
	iterator := iterable.Iterate()
	defer iterator.Done()
	var item starlark.Value
	values := []string{}
	for iterator.Next(&item) {
		if len(values) >= max {
			return nil, errors.New("item count exceeds limit")
		}
		s, ok := starlark.AsString(item)
		if !ok {
			return nil, errors.New("expected a list of strings")
		}
		values = append(values, s)
	}
	return values, nil
}
