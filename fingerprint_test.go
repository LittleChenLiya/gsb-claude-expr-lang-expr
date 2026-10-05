package expr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/internal/testify/assert"
	"github.com/expr-lang/expr/internal/testify/require"
	"github.com/expr-lang/expr/vm"
)

// Two environments with the same variable name but different static types.
type fingerprintEnvV1 struct {
	Limit int
}

type fingerprintEnvV2 struct {
	Limit string
}

// ExampleCheck demonstrates how a compiled program carries a fingerprint of
// the environment it was compiled against, and how expr.Check verifies a
// (possibly different) environment before running the program.
func ExampleCheck() {
	program, err := expr.Compile(`Limit > 10`, expr.Env(fingerprintEnvV1{}))
	if err != nil {
		panic(err)
	}

	// The fingerprint records the static variable type, function
	// signatures, patch options and the compile mode; never values.
	fp := program.EnvFingerprint()
	fmt.Println(fp.Variables["Limit"])

	// Same static type: compatible, even though the value differs.
	fmt.Println(expr.Check(program, fingerprintEnvV1{Limit: 42}) == nil)

	// Different static type: rejected before running.
	fmt.Println(expr.Check(program, fingerprintEnvV2{Limit: "high"}) != nil)

	// Output:
	// int
	// true
	// true
}

func TestFingerprint_value_change_still_runs(t *testing.T) {
	program, err := expr.Compile(`x + 1`, expr.Env(map[string]any{"x": 1}))
	require.NoError(t, err)

	// Dynamic values are not part of the fingerprint: 1 -> 2 is fine.
	out, err := expr.Run(program, map[string]any{"x": 2})
	require.NoError(t, err)
	require.Equal(t, 3, out)
}

func TestFingerprint_variable_type_change_rejected(t *testing.T) {
	program, err := expr.Compile(`x + 1`, expr.Env(map[string]any{"x": 1}))
	require.NoError(t, err)

	// int -> string triggers an incompatibility error before running.
	_, err = expr.Run(program, map[string]any{"x": "2"})
	require.Error(t, err)

	var compatErr *expr.EnvCompatibilityError
	require.True(t, errors.As(err, &compatErr), "expected *expr.EnvCompatibilityError, got %T", err)
	require.Contains(t, err.Error(), `"x"`)
	require.Contains(t, err.Error(), "string")
	require.Contains(t, err.Error(), "int")
}

func TestFingerprint_function_signature_change_rejected(t *testing.T) {
	calls := 0

	program, err := expr.Compile(`f(21)`, expr.Env(map[string]any{
		"f": func(x int) int { return x * 2 },
	}))
	require.NoError(t, err)

	// Same name, different signature: rejected before running, and the
	// host function is never executed by the pre-flight check.
	_, err = expr.Run(program, map[string]any{
		"f": func(x string) string { calls++; return x },
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `"f"`)
	require.Contains(t, err.Error(), "func(string) string")
	require.Contains(t, err.Error(), "func(int) int")
	require.Equal(t, 0, calls, "pre-flight check must not execute host functions")

	// Same signature: runs as before.
	out, err := expr.Run(program, map[string]any{
		"f": func(x int) int { calls++; return x },
	})
	require.NoError(t, err)
	require.Equal(t, 21, out)
	require.Equal(t, 1, calls)
}

func TestFingerprint_old_program_rejected_in_new_environment(t *testing.T) {
	program, err := expr.Compile(`Limit > 10`, expr.Env(fingerprintEnvV1{}))
	require.NoError(t, err)

	// The old program must not run by mistake in the new environment.
	_, err = expr.Run(program, fingerprintEnvV2{Limit: "high"})
	require.Error(t, err)
	require.Contains(t, err.Error(), `"Limit"`)

	// It still runs in the environment it was compiled against.
	out, err := expr.Run(program, fingerprintEnvV1{Limit: 42})
	require.NoError(t, err)
	require.Equal(t, true, out)
}

func TestFingerprint_dynamic_type_not_treated_as_verified(t *testing.T) {
	program, err := expr.Compile(`x`, expr.Env(map[string]any{"x": nil}))
	require.NoError(t, err)

	fp := program.EnvFingerprint()
	require.NotNil(t, fp)

	// An unresolved dynamic type must be recorded as such (empty string),
	// never as an already verified static type.
	typ, ok := fp.Variables["x"]
	require.True(t, ok, "x should be recorded in the fingerprint")
	require.Equal(t, "", typ)

	// Dynamic references are left to the runtime: any value type works.
	out, err := expr.Run(program, map[string]any{"x": 1})
	require.NoError(t, err)
	require.Equal(t, 1, out)

	out, err = expr.Run(program, map[string]any{"x": "two"})
	require.NoError(t, err)
	require.Equal(t, "two", out)
}

func TestFingerprint_undefined_variable_not_treated_as_verified(t *testing.T) {
	program, err := expr.Compile(`x + 1`, expr.AllowUndefinedVariables())
	require.NoError(t, err)

	fp := program.EnvFingerprint()
	require.NotNil(t, fp)
	typ, ok := fp.Variables["x"]
	require.True(t, ok, "undefined variable should be recorded as a dynamic reference")
	require.Equal(t, "", typ)

	out, err := expr.Run(program, map[string]any{"x": 41})
	require.NoError(t, err)
	require.Equal(t, 42, out)
}

func TestFingerprint_records_options_and_mode(t *testing.T) {
	program, err := expr.Compile(`double(x)`,
		expr.Env(map[string]any{"x": 1}),
		expr.AsInt(),
		expr.Optimize(false),
		expr.Function("double",
			func(params ...any) (any, error) { return params[0].(int) * 2, nil },
			new(func(int) int),
		),
	)
	require.NoError(t, err)

	fp := program.EnvFingerprint()
	require.NotNil(t, fp)
	assert.Contains(t, fp.Mode, "optimize=false")
	assert.Contains(t, fp.Mode, "expect=int")
	assert.Contains(t, fp.Options, "function:double:func(int) int")

	// The fingerprint is a stable string usable as a cache key next to the
	// expression text.
	other, err := expr.Compile(`double(x)`,
		expr.Env(map[string]any{"x": 1}),
		expr.AsInt(),
		expr.Optimize(false),
		expr.Function("double",
			func(params ...any) (any, error) { return params[0].(int) * 2, nil },
			new(func(int) int),
		),
	)
	require.NoError(t, err)
	require.Equal(t, fp.String(), other.EnvFingerprint().String())

	// A program compiled with different options has a different fingerprint.
	different, err := expr.Compile(`double(x)`,
		expr.Env(map[string]any{"x": 1}),
		expr.Function("double",
			func(params ...any) (any, error) { return params[0].(int) * 2, nil },
			new(func(int) int),
		),
	)
	require.NoError(t, err)
	require.NotEqual(t, fp.String(), different.EnvFingerprint().String())

	out, err := expr.Run(program, map[string]any{"x": 21})
	require.NoError(t, err)
	require.Equal(t, 42, out)
}

func TestFingerprint_absent_variable_left_to_runtime(t *testing.T) {
	// Absence is a runtime concern, not a type incompatibility: optional
	// chaining may never touch the variable.
	program, err := expr.Compile(`foo?.bar ?? "default"`, expr.Env(map[string]any{
		"foo": map[string]any{"bar": "value"},
	}))
	require.NoError(t, err)

	out, err := expr.Run(program, map[string]any{})
	require.NoError(t, err)
	require.Equal(t, "default", out)

	// Without optional chaining the runtime reports the missing variable.
	program, err = expr.Compile(`x + 1`, expr.Env(map[string]any{"x": 1}))
	require.NoError(t, err)
	_, err = expr.Run(program, map[string]any{})
	require.Error(t, err)
}

func TestFingerprint_check_without_fingerprint(t *testing.T) {
	// Programs compiled without expr.Compile options carry no fingerprint
	// and are always accepted by Check.
	program, err := expr.Compile(`1 + 1`)
	require.NoError(t, err)
	require.NotNil(t, program.EnvFingerprint())

	out, err := expr.Run(program, nil)
	require.NoError(t, err)
	require.Equal(t, 2, out)

	plain := vm.Program{}
	require.NoError(t, expr.Check(&plain, map[string]any{"x": 1}))
}
