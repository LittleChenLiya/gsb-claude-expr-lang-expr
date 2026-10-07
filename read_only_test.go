package expr_test

import (
	"testing"

	"github.com/expr-lang/expr/internal/testify/require"

	"github.com/expr-lang/expr"
)

// readOnlyRecord is a concrete (statically known) environment object: it
// exposes both a getter method and a plain field.
type readOnlyRecord struct {
	Label string
}

func (r readOnlyRecord) GetLabel() string { return "method:" + r.Label }

// readOnlyItem is implemented by readOnlyRecord, so a value of the struct
// can also be passed around wrapped in a (non-any) interface.
type readOnlyItem interface {
	GetLabel() string
}

// readOnlyEnv is the environment for read-only mode checks. Item is typed as
// an interface, so expressions see an interface-wrapped value; Record is
// statically a struct. EnvMethod is a method defined on the environment
// itself.
type readOnlyEnv struct {
	Record readOnlyRecord
	Item   readOnlyItem
}

func (readOnlyEnv) EnvMethod() string { return "env-method" }

func TestReadOnly(t *testing.T) {
	env := readOnlyEnv{
		Record: readOnlyRecord{Label: "rec"},
		Item:   readOnlyRecord{Label: "itm"},
	}

	// A function explicitly registered with expr.Function must stay
	// available in read-only mode.
	pure := expr.Function("upper",
		func(params ...any) (any, error) {
			s := params[0].(string)
			out := make([]rune, 0, len(s))
			for _, r := range s {
				if r >= 'a' && r <= 'z' {
					r -= 'a' - 'A'
				}
				out = append(out, r)
			}
			return string(out), nil
		},
		new(func(string) string),
	)

	readOnlyOpts := []expr.Option{expr.Env(readOnlyEnv{}), expr.ReadOnly(), pure}

	compileOK := func(name, code string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			program, err := expr.Compile(code, readOnlyOpts...)
			require.NoError(t, err)
			out, err := expr.Run(program, env)
			require.NoError(t, err)
			t.Logf("%s => %v", code, out)
		})
	}

	compileRejected := func(name, code, wantErr string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			_, err := expr.Compile(code, readOnlyOpts...)
			require.Error(t, err)
			require.Contains(t, err.Error(), wantErr)
			t.Logf("%s => rejected: %v", code, err)
		})
	}

	// Read-only mode: field reads and explicitly registered pure functions
	// succeed for both the struct and the interface-wrapped value.
	compileOK("struct field read", `Record.Label`)
	compileOK("interface field read", `Item.Label`)
	compileOK("pure function on struct field", `upper(Record.Label)`)
	compileOK("pure function on interface field", `upper(Item.Label)`)

	// Read-only mode: member methods are rejected at Compile for both the
	// static struct type and the interface-wrapped value, as well as on the
	// environment itself.
	compileRejected("struct method call", `Record.GetLabel()`, "read-only mode")
	compileRejected("interface method call", `Item.GetLabel()`, "read-only mode")
	compileRejected("env method call", `EnvMethod()`, "read-only mode")

	// Default mode (without ReadOnly): methods remain callable on both the
	// struct and the interface-wrapped value, and on the environment itself.
	defaultOpts := []expr.Option{expr.Env(readOnlyEnv{}), pure}
	defaultCall := func(name, code, want string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			program, err := expr.Compile(code, defaultOpts...)
			require.NoError(t, err)
			out, err := expr.Run(program, env)
			require.NoError(t, err)
			require.Equal(t, want, out)
			t.Logf("%s => %v", code, out)
		})
	}
	defaultCall("default struct method call", `Record.GetLabel()`, "method:rec")
	defaultCall("default interface method call", `Item.GetLabel()`, "method:itm")
	defaultCall("default env method call", `EnvMethod()`, "env-method")
}
