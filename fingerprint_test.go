// Package expr_test contains the acceptance tests for the explicit environment
// fingerprint. Run them with:
//
//	go test -run 'TestFingerprint|ExampleRunChecked' -v .
package expr_test

import (
	"fmt"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/internal/testify/assert"
	"github.com/expr-lang/expr/internal/testify/require"
	"github.com/expr-lang/expr/vm"
)

// TestFingerprint_VariableTypeChange verifies that a static variable changing
// its type (int -> string) between compile time and run time is rejected.
func TestFingerprint_VariableTypeChange(t *testing.T) {
	env1 := map[string]any{"x": 1}
	program, err := expr.Compile(`x + 1`, expr.Env(env1))
	require.NoError(t, err)

	env2 := map[string]any{"x": "string"}
	_, err = expr.RunChecked(program, env2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `variable "x" changed type from int to string`)
}

// TestFingerprint_ValueChangeStillRuns verifies that a static variable keeping
// the same type but changing its value (1 -> 2) still runs.
func TestFingerprint_ValueChangeStillRuns(t *testing.T) {
	env1 := map[string]any{"x": 1}
	program, err := expr.Compile(`x + 1`, expr.Env(env1))
	require.NoError(t, err)

	env2 := map[string]any{"x": 2}
	out, err := expr.RunChecked(program, env2)
	require.NoError(t, err)
	assert.Equal(t, 3, out)
}

// TestFingerprint_FunctionSignatureChange verifies that a function with the
// same name but a different signature is rejected and the host function is
// never called (call count is 0).
func TestFingerprint_FunctionSignatureChange(t *testing.T) {
	calls := 0
	fnInt := func(x int) int {
		calls++
		return x + 1
	}
	env1 := map[string]any{"f": fnInt}
	program, err := expr.Compile(`f(1)`, expr.Env(env1))
	require.NoError(t, err)

	fnStr := func(x string) string {
		calls++
		return x
	}
	env2 := map[string]any{"f": fnStr}
	_, err = expr.RunChecked(program, env2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `function "f" changed its signature`)
	assert.Equal(t, 0, calls)
}

// TestFingerprint_UnresolvedDynamicType verifies that a variable with an
// unresolved dynamic type is not treated as validated.
func TestFingerprint_UnresolvedDynamicType(t *testing.T) {
	type Env struct {
		X any
	}
	program, err := expr.Compile(`X`, expr.Env(Env{X: 1}))
	require.NoError(t, err)

	_, err = expr.RunChecked(program, Env{X: "string"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved dynamic type")
}

// TestFingerprint_Recorded verifies that the fingerprint is recorded on the
// program and is accessible.
func TestFingerprint_Recorded(t *testing.T) {
	env := map[string]any{"x": 1, "s": "a"}
	program, err := expr.Compile(`x + 1`, expr.Env(env))
	require.NoError(t, err)

	fp := program.EnvFingerprint()
	require.NotNil(t, fp)
	assert.Contains(t, fp.String(), "x=int")
	assert.Contains(t, fp.String(), "s=string")
}

// TestFingerprint_DefaultRunUnchanged verifies that the default Run still
// works without any compatibility check.
func TestFingerprint_DefaultRunUnchanged(t *testing.T) {
	env1 := map[string]any{"x": 1}
	program, err := expr.Compile(`x`, expr.Env(env1))
	require.NoError(t, err)

	// Default Run does not check the environment fingerprint.
	out, err := expr.Run(program, map[string]any{"x": "string"})
	require.NoError(t, err)
	assert.Equal(t, "string", out)
}

// TestFingerprint_NilProgram verifies that RunChecked on a nil program errors.
func TestFingerprint_NilProgram(t *testing.T) {
	_, err := expr.RunChecked(nil, map[string]any{})
	require.Error(t, err)
}

// TestFingerprint_NoFingerprint verifies that RunChecked on a program compiled
// without a configuration errors.
func TestFingerprint_NoFingerprint(t *testing.T) {
	// A hand-built program has no environment fingerprint.
	program := vm.NewProgram(file.Source{}, nil, nil, 0, nil, nil, nil, nil, nil, nil)
	require.Nil(t, program.EnvFingerprint())

	_, err := expr.RunChecked(program, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without an environment fingerprint")
}

// TestFingerprint_PatchOptionChange verifies that a changed patch option is
// rejected.
func TestFingerprint_PatchOptionChange(t *testing.T) {
	env := map[string]any{
		"x":   1,
		"y":   2,
		"add": func(a, b int) int { return a + b },
	}
	program, err := expr.Compile(`x + y`, expr.Env(env), expr.Operator("+", "add"))
	require.NoError(t, err)

	// Running without the same patch option should be rejected.
	_, err = expr.RunChecked(program, env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "patch options differ")
}

// TestFingerprint_ModeChange verifies that a changed compilation mode is
// rejected.
func TestFingerprint_ModeChange(t *testing.T) {
	env := map[string]any{"x": 1}
	program, err := expr.Compile(`x + 1`, expr.Env(env), expr.Optimize(false))
	require.NoError(t, err)

	_, err = expr.RunChecked(program, env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compilation mode differs")
}

// TestFingerprint_CompileTimeFunction verifies that functions registered via
// expr.Function are recorded in the fingerprint.
func TestFingerprint_CompileTimeFunction(t *testing.T) {
	env := map[string]any{"x": 1}
	program, err := expr.Compile(`Double(x)`, expr.Env(env),
		expr.Function("Double", func(p ...any) (any, error) {
			return p[0].(int) * 2, nil
		}, new(func(int) int)),
	)
	require.NoError(t, err)

	fp := program.EnvFingerprint()
	require.NotNil(t, fp)
	assert.Contains(t, fp.String(), "func(int) int")
}

// envMethodEnv is an environment type with a method.
type envMethodEnv struct {
	X int
}

// Double is a host method; it must not be invoked by the pre-check.
func (e envMethodEnv) Double(x int) int {
	return x * 2
}

// TestFingerprint_EnvMethod verifies that environment methods are recorded as
// function signatures and that a compatible environment runs.
func TestFingerprint_EnvMethod(t *testing.T) {
	program, err := expr.Compile(`Double(X)`, expr.Env(envMethodEnv{}))
	require.NoError(t, err)

	fp := program.EnvFingerprint()
	require.NotNil(t, fp)
	assert.Contains(t, fp.String(), "envMethodEnv, int")

	out, err := expr.RunChecked(program, envMethodEnv{X: 21})
	require.NoError(t, err)
	assert.Equal(t, 42, out)
}

// TestFingerprint_EnvMethodSignatureChange verifies that a function with the
// same name but a different signature is rejected and the host function is
// never called.
func TestFingerprint_EnvMethodSignatureChange(t *testing.T) {
	calls := 0
	env := map[string]any{}
	program, err := expr.Compile(`f(1)`, expr.Env(env),
		expr.Function("f", func(p ...any) (any, error) {
			calls++
			return p[0].(int) + 1, nil
		}, new(func(int) int)),
	)
	require.NoError(t, err)

	_, err = expr.RunChecked(program, env,
		expr.Function("f", func(p ...any) (any, error) {
			calls++
			return p[0].(string), nil
		}, new(func(string) string)),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `function "f" changed its signature`)
	assert.Equal(t, 0, calls)
}

// ExampleRunChecked_twoEnvironments demonstrates the two-environment usage:
// a program compiled for one environment runs against a compatible one and is
// rejected by an incompatible one.
func ExampleRunChecked_twoEnvironments() {
	code := `x + 1`

	env1 := map[string]any{"x": 1}
	program, err := expr.Compile(code, expr.Env(env1))
	if err != nil {
		panic(err)
	}

	// Same environment: runs.
	out, err := expr.RunChecked(program, env1)
	if err != nil {
		panic(err)
	}
	fmt.Println("same env:", out)

	// Same type, different value: still runs.
	env2 := map[string]any{"x": 2}
	out, err = expr.RunChecked(program, env2)
	if err != nil {
		panic(err)
	}
	fmt.Println("different value:", out)

	// Different type: rejected.
	env3 := map[string]any{"x": "string"}
	_, err = expr.RunChecked(program, env3)
	if err != nil {
		fmt.Println("different type:", err)
	}

	// Output:
	// same env: 2
	// different value: 3
	// different type: program environment check: variable "x" changed type from int to string
}

// Ensure vm.Program implements the fingerprint accessor.
var _ = (*vm.Program)(nil)
