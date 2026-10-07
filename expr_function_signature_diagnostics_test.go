package expr_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/internal/testify/require"
)

// diagFixture holds explicitly typed functions with call counters, so tests
// can prove that Compile performs a purely static check (zero calls).
type diagFixture struct {
	addCalls   int
	sumCalls   int
	failCalls  int
}

func (f *diagFixture) options() []expr.Option {
	return []expr.Option{
		// Fixed two-argument function with explicitly registered types.
		expr.Function("add", func(params ...any) (any, error) {
			f.addCalls++
			return params[0].(int) + params[1].(int), nil
		}, new(func(int, int) int)),
		// Variadic function: diagnostics must stay the original ones.
		expr.Function("sum", func(params ...any) (any, error) {
			f.sumCalls++
			total := 0
			for _, p := range params {
				total += p.(int)
			}
			return total, nil
		}, new(func(...int) int)),
		// Function which always fails at runtime.
		expr.Function("fail", func(params ...any) (any, error) {
			f.failCalls++
			return nil, errors.New("boom: runtime failure")
		}, new(func(int) (int, error))),
	}
}

// Case 1: fixed two-argument function called with a wrong argument type.
// The compile error must point at the second argument, name its position
// and the expected type, and the function must not be called.
func TestFunctionSignatureDiagnostics_WrongType(t *testing.T) {
	fx := &diagFixture{}
	ops := append(fx.options(), expr.FunctionSignatureDiagnostics())

	program, err := expr.Compile(`add(1, "two")`, ops...)
	require.Error(t, err)
	require.Nil(t, program)

	msg := err.Error()
	require.Contains(t, msg, "invalid argument 2 in call to add: expected int, got string")
	// The error location points exactly at the offending argument `"two"`.
	require.Contains(t, msg, "(1:8)")
	require.Contains(t, msg, "add(1, \"two\")")
	// Static check: the function was never invoked during Compile.
	require.Equal(t, 0, fx.addCalls)
}

// Case 2: fixed two-argument function called correctly compiles and runs;
// the function is called only at runtime, exactly once.
func TestFunctionSignatureDiagnostics_CorrectCall(t *testing.T) {
	fx := &diagFixture{}
	ops := append(fx.options(), expr.FunctionSignatureDiagnostics())

	program, err := expr.Compile(`add(1, 2)`, ops...)
	require.NoError(t, err)
	require.Equal(t, 0, fx.addCalls, "Compile must not call the function")

	out, err := expr.Run(program, nil)
	require.NoError(t, err)
	require.Equal(t, 3, out)
	require.Equal(t, 1, fx.addCalls, "Run calls the function exactly once")
}

// Case 3: variadic function keeps the original allowed behavior, both for
// valid calls and for the original error message on a type mismatch.
func TestFunctionSignatureDiagnostics_Variadic(t *testing.T) {
	fx := &diagFixture{}
	ops := append(fx.options(), expr.FunctionSignatureDiagnostics())

	// Any number of arguments is still allowed.
	program, err := expr.Compile(`sum(1, 2, 3)`, ops...)
	require.NoError(t, err)
	require.Equal(t, 0, fx.sumCalls)

	out, err := expr.Run(program, nil)
	require.NoError(t, err)
	require.Equal(t, 6, out)
	require.Equal(t, 1, fx.sumCalls)

	// Type mismatch inside variadic arguments keeps the original message.
	_, err = expr.Compile(`sum(1, "two")`, ops...)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot use string as argument (type int) to call sum")
	require.Equal(t, 1, fx.sumCalls, "Compile must not call the variadic function")
}

// Runtime errors returned by the function itself are reported by Run and
// are never disguised as compile errors.
func TestFunctionSignatureDiagnostics_RuntimeError(t *testing.T) {
	fx := &diagFixture{}
	ops := append(fx.options(), expr.FunctionSignatureDiagnostics())

	program, err := expr.Compile(`fail(1)`, ops...)
	require.NoError(t, err, "runtime failures must not surface as compile errors")
	require.Equal(t, 0, fx.failCalls)

	_, err = expr.Run(program, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom: runtime failure")
	require.Equal(t, 1, fx.failCalls)
}

// Without the new option the original interface and error messages are
// preserved unchanged.
func TestFunctionSignatureDiagnostics_Disabled(t *testing.T) {
	fx := &diagFixture{}

	_, err := expr.Compile(`add(1, "two")`, fx.options()...)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot use string as argument (type int) to call add")
	require.NotContains(t, err.Error(), "invalid argument 2")
	require.Equal(t, 0, fx.addCalls)
}

// Typed functions from the environment (reflect.Func) get the same early
// diagnostics when the option is enabled.
func TestFunctionSignatureDiagnostics_EnvFunction(t *testing.T) {
	type Env struct{ Mul func(int, int) int }
	calls := 0
	env := Env{Mul: func(a, b int) int { calls++; return a * b }}

	_, err := expr.Compile(`Mul(1, "two")`, expr.Env(Env{}), expr.FunctionSignatureDiagnostics())
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid argument 2 in call to Mul: expected int, got string")
	require.Equal(t, 0, calls)

	program, err := expr.Compile(`Mul(3, 4)`, expr.Env(Env{}), expr.FunctionSignatureDiagnostics())
	require.NoError(t, err)
	out, err := expr.Run(program, env)
	require.NoError(t, err)
	require.Equal(t, 12, out)
	require.Equal(t, 1, calls)
}

// Temp-dir fixture: renders the diagnostics of all cases into a small
// report file, demonstrating the results with a file-based fixture.
func TestFunctionSignatureDiagnostics_ReportFixture(t *testing.T) {
	fx := &diagFixture{}
	ops := append(fx.options(), expr.FunctionSignatureDiagnostics())

	var b strings.Builder
	write := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	_, err := expr.Compile(`add(1, "two")`, ops...)
	write("wrong-type compile error: %v", err)

	program, err := expr.Compile(`add(40, 2)`, ops...)
	require.NoError(t, err)
	out, err := expr.Run(program, nil)
	require.NoError(t, err)
	write("correct call result: %v (add calls so far: %d)", out, fx.addCalls)

	program, err = expr.Compile(`sum(1, 2, 3)`, ops...)
	require.NoError(t, err)
	out, err = expr.Run(program, nil)
	require.NoError(t, err)
	write("variadic call result: %v (sum calls so far: %d)", out, fx.sumCalls)

	program, err = expr.Compile(`fail(1)`, ops...)
	require.NoError(t, err)
	_, err = expr.Run(program, nil)
	write("runtime error: %v (fail calls so far: %d)", err, fx.failCalls)

	dir := t.TempDir()
	report := filepath.Join(dir, "diagnostics.txt")
	require.NoError(t, os.WriteFile(report, []byte(b.String()), 0o600))

	content, err := os.ReadFile(report)
	require.NoError(t, err)
	t.Logf("report fixture %s:\n%s", report, content)

	require.Contains(t, string(content), "invalid argument 2 in call to add: expected int, got string")
	require.Contains(t, string(content), "correct call result: 42 (add calls so far: 1)")
	require.Contains(t, string(content), "variadic call result: 6 (sum calls so far: 1)")
	require.Contains(t, string(content), "boom: runtime failure")
	require.Contains(t, string(content), "(fail calls so far: 1)")
}
