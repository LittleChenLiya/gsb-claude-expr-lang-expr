package expr_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/internal/testify/assert"
	"github.com/expr-lang/expr/internal/testify/require"
)

// Fixed two-argument function used across the diagnostics tests.
// calls counts invocations to prove the compile-time check never calls it.
type diagCounter struct {
	calls int
}

func (c *diagCounter) greet(params ...any) (any, error) {
	c.calls++
	return fmt.Sprintf("%v:%v", params[0], params[1]), nil
}

var greetSig = func(a int, b string) string { return "" }

func TestFunctionArgumentDiagnostics_wrong_type(t *testing.T) {
	counter := &diagCounter{}
	input := "greet(1, 2)"

	_, err := expr.Compile(input,
		expr.Function("greet", counter.greet, greetSig),
		expr.FunctionArgumentDiagnostics(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid argument 2 to call greet")
	assert.Contains(t, err.Error(), "expected string")
	assert.Contains(t, err.Error(), "got int")

	// The error location must point at the second argument.
	var fileErr *file.Error
	require.ErrorAs(t, err, &fileErr)
	assert.Equal(t, strings.Index(input, "2"), fileErr.From,
		"error location must point at the offending argument")

	// The compile-time check must not call the function.
	assert.Equal(t, 0, counter.calls, "function must not be called during compile")
}

func TestFunctionArgumentDiagnostics_correct_call(t *testing.T) {
	counter := &diagCounter{}

	program, err := expr.Compile(`greet(1, "a")`,
		expr.Function("greet", counter.greet, greetSig),
		expr.FunctionArgumentDiagnostics(),
	)
	require.NoError(t, err)
	assert.Equal(t, 0, counter.calls, "function must not be called during compile")

	out, err := expr.Run(program, nil)
	require.NoError(t, err)
	assert.Equal(t, "1:a", out)
	assert.Equal(t, 1, counter.calls, "function is called exactly once at runtime")
}

func TestFunctionArgumentDiagnostics_variadic(t *testing.T) {
	calls := 0
	sum := func(params ...any) (any, error) {
		calls++
		total := 0
		for _, p := range params {
			total += p.(int)
		}
		return total, nil
	}
	sumSig := func(xs ...int) int { return 0 }

	// Variadic calls keep the original allowed behavior.
	program, err := expr.Compile("sum(1, 2, 3)",
		expr.Function("sum", sum, sumSig),
		expr.FunctionArgumentDiagnostics(),
	)
	require.NoError(t, err)
	assert.Equal(t, 0, calls)

	out, err := expr.Run(program, nil)
	require.NoError(t, err)
	assert.Equal(t, 6, out)

	// A type mismatch inside the variadic tail keeps the original message.
	_, err = expr.Compile(`sum(1, "x")`,
		expr.Function("sum", sum, sumSig),
		expr.FunctionArgumentDiagnostics(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use string as argument (type int) to call sum")
	assert.NotContains(t, err.Error(), "invalid argument")
	assert.Equal(t, 1, calls, "function must not be called during compile")
}

func TestFunctionArgumentDiagnostics_runtime_error(t *testing.T) {
	boom := func(params ...any) (any, error) {
		return nil, errors.New("boom")
	}

	// A function that fails at runtime still compiles: the runtime
	// exception is not disguised as a compile error.
	program, err := expr.Compile(`boom("x")`,
		expr.Function("boom", boom, func(s string) string { return "" }),
		expr.FunctionArgumentDiagnostics(),
	)
	require.NoError(t, err, "runtime failure must not surface as compile error")

	_, err = expr.Run(program, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestFunctionArgumentDiagnostics_disabled_keeps_original(t *testing.T) {
	counter := &diagCounter{}

	// Without the new option, the original interface and message are kept.
	_, err := expr.Compile("greet(1, 2)",
		expr.Function("greet", counter.greet, greetSig),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use int as argument (type string) to call greet")
	assert.NotContains(t, err.Error(), "invalid argument")
	assert.Equal(t, 0, counter.calls)
}

func TestFunctionArgumentDiagnostics_env_function(t *testing.T) {
	type Env struct {
		Mix func(a int, b string) string
	}

	_, err := expr.Compile("Mix(1, 2)",
		expr.Env(Env{}),
		expr.FunctionArgumentDiagnostics(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid argument 2 to call Mix")
	assert.Contains(t, err.Error(), "expected string, got int")
}

// TestFunctionArgumentDiagnostics_report demonstrates the diagnostics with a
// small temporary-directory fixture: the compile error report is written to
// a file and verified.
func TestFunctionArgumentDiagnostics_report(t *testing.T) {
	counter := &diagCounter{}
	input := "greet(1, 2)"

	_, err := expr.Compile(input,
		expr.Function("greet", counter.greet, greetSig),
		expr.FunctionArgumentDiagnostics(),
	)
	require.Error(t, err)

	report := filepath.Join(t.TempDir(), "report.txt")
	require.NoError(t, os.WriteFile(report, []byte(err.Error()), 0o600))

	content, err := os.ReadFile(report)
	require.NoError(t, err)
	t.Logf("diagnostic report:\n%s", string(content))
	assert.Contains(t, string(content), "invalid argument 2 to call greet: expected string, got int")
	assert.Equal(t, 0, counter.calls)
}
