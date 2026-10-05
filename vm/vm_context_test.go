package vm_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/internal/testify/require"
	"github.com/expr-lang/expr/vm"
)

// TestRunWithContext_CancelBeforeThirdHostCall verifies that cancelling the
// context before the third host call results in exactly two host calls.
func TestRunWithContext_CancelBeforeThirdHostCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	env := map[string]any{
		"items": []int{1, 2, 3, 4, 5},
		"visit": func(i int) int {
			calls++
			if calls == 2 {
				cancel() // cancel before the third host call
			}
			return i
		},
	}

	program, err := expr.Compile(`map(items, visit(#))`, expr.Env(env))
	require.NoError(t, err)

	v := &vm.VM{}
	_, err = v.RunWithContext(ctx, program, env)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got %v", err)
	require.Equal(t, 2, calls, "third host call must not be entered")

	var cancelled *vm.CancelledError
	require.True(t, errors.As(err, &cancelled), "expected *vm.CancelledError, got %T", err)
}

// TestRunWithContext_NestedLoops verifies that nested iterations (map inside
// map) observe the same cancellation signal and stop as well.
func TestRunWithContext_NestedLoops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	env := map[string]any{
		"outer": []int{1, 2, 3, 4, 5},
		"inner": []int{1, 2, 3, 4, 5},
		"visit": func(i int) int {
			calls++
			if calls == 3 {
				cancel()
			}
			return i
		},
	}

	program, err := expr.Compile(`map(outer, map(inner, visit(#)))`, expr.Env(env))
	require.NoError(t, err)

	_, err = vm.RunWithContext(ctx, program, env)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got %v", err)
	require.Equal(t, 3, calls, "nested loops must stop at the cancellation boundary")
}

// TestRunWithContext_ReuseVMAfterCancel verifies that the VM state is
// recovered after a cancellation and the same VM can be reused with a fresh
// context (or none) without observing the old cancellation signal.
func TestRunWithContext_ReuseVMAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	env := map[string]any{
		"items": []int{1, 2, 3, 4, 5},
		"visit": func(i int) int {
			calls++
			if calls == 2 {
				cancel()
			}
			return i
		},
	}

	program, err := expr.Compile(`map(items, visit(#))`, expr.Env(env))
	require.NoError(t, err)

	v := &vm.VM{}
	_, err = v.RunWithContext(ctx, program, env)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Equal(t, 2, calls)

	// Reuse the same VM with a fresh context: no old signal is observed.
	calls = 0
	freshEnv := map[string]any{
		"items": []int{1, 2, 3, 4, 5},
		"visit": func(i int) int {
			calls++
			return i
		},
	}
	out, err := v.RunWithContext(context.Background(), program, freshEnv)
	require.NoError(t, err)
	require.Equal(t, []any{1, 2, 3, 4, 5}, out)
	require.Equal(t, 5, calls)

	// Reuse the same VM without any context at all.
	calls = 0
	out, err = v.Run(program, freshEnv)
	require.NoError(t, err)
	require.Equal(t, []any{1, 2, 3, 4, 5}, out)
	require.Equal(t, 5, calls)
}

// TestRunWithContext_ErrorPosition verifies that the cancellation error
// carries the instruction pointer and the source location taken from the
// compile-time location mapping of the program.
func TestRunWithContext_ErrorPosition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := map[string]any{
		"items": []int{1, 2, 3, 4, 5},
		"visit": func(i int) int {
			cancel()
			return i
		},
	}

	program, err := expr.Compile(`map(items, visit(#))`, expr.Env(env))
	require.NoError(t, err)

	_, err = vm.RunWithContext(ctx, program, env)
	require.Error(t, err)

	var cancelled *vm.CancelledError
	require.True(t, errors.As(err, &cancelled), "expected *vm.CancelledError, got %T", err)
	require.True(t, cancelled.IP >= 0 && cancelled.IP < len(program.Locations()),
		"IP %d out of locations range %d", cancelled.IP, len(program.Locations()))
	require.Equal(t, program.Locations()[cancelled.IP], cancelled.Location,
		"location must come from the compile-time mapping")

	var fileErr *file.Error
	require.True(t, errors.As(err, &fileErr), "expected *file.Error, got %T", err)
	require.Contains(t, err.Error(), "execution cancelled at instruction")
}

// TestRunWithContext_AlreadyCancelled verifies that no host call is entered
// when the context is cancelled before the run starts.
func TestRunWithContext_AlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	env := map[string]any{
		"items": []int{1, 2, 3, 4, 5},
		"visit": func(i int) int {
			calls++
			return i
		},
	}

	program, err := expr.Compile(`map(items, visit(#))`, expr.Env(env))
	require.NoError(t, err)

	_, err = vm.RunWithContext(ctx, program, env)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Equal(t, 0, calls)
}

// TestRunWithContext_NilContext verifies that a nil context behaves exactly
// like a plain Run.
func TestRunWithContext_NilContext(t *testing.T) {
	env := map[string]any{"items": []int{1, 2, 3, 4, 5}}

	program, err := expr.Compile(`map(items, # * 2)`, expr.Env(env))
	require.NoError(t, err)

	out, err := vm.RunWithContext(nil, program, env) //nolint:staticcheck
	require.NoError(t, err)
	require.Equal(t, []any{2, 4, 6, 8, 10}, out)
}

// TestRunWithContext_NilProgram verifies the package-level helper guards
// against a nil program.
func TestRunWithContext_NilProgram(t *testing.T) {
	_, err := vm.RunWithContext(context.Background(), nil, nil)
	require.Error(t, err)
}

// ExampleRunWithContext maps over a five-item list with a context.
func ExampleRunWithContext() {
	env := map[string]any{
		"items": []int{1, 2, 3, 4, 5},
	}
	program, err := expr.Compile(`map(items, # * 2)`, expr.Env(env))
	if err != nil {
		panic(err)
	}
	out, err := vm.RunWithContext(context.Background(), program, env)
	if err != nil {
		panic(err)
	}
	fmt.Println(out)
	// Output: [2 4 6 8 10]
}

// ExampleVM_RunWithContext cancels a run over a five-item list before the
// third host call, then reuses the same VM with a fresh context.
func ExampleVM_RunWithContext() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	env := map[string]any{
		"items": []int{1, 2, 3, 4, 5},
		"visit": func(i int) int {
			calls++
			if calls == 2 {
				cancel() // cancel before the third host call
			}
			return i
		},
	}

	program, err := expr.Compile(`map(items, visit(#))`, expr.Env(env))
	if err != nil {
		panic(err)
	}

	v := &vm.VM{}
	_, err = v.RunWithContext(ctx, program, env)
	fmt.Println(errors.Is(err, context.Canceled), calls)

	// The same VM can be reused afterwards; the old cancellation signal
	// is not observed.
	calls = 0
	out, err := v.RunWithContext(context.Background(), program, map[string]any{
		"items": []int{1, 2, 3, 4, 5},
		"visit": func(i int) int {
			calls++
			return i
		},
	})
	fmt.Println(out, err, calls)
	// Output:
	// true 2
	// [1 2 3 4 5] <nil> 5
}
