package vm

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/conf"
)

// EnvFingerprint is an explicit fingerprint of the environment a Program was
// compiled for. It records static variable types, function signatures, patch
// options and the compilation mode. Dynamic values are not part of the
// fingerprint: variables whose static type cannot be determined are marked as
// unresolved and are never treated as validated.
//
// The fingerprint is computed from reflection only; host functions are never
// invoked.
type EnvFingerprint struct {
	variables map[string]varFingerprint
	functions map[string]funcFingerprint
	patches   []string
	mode      modeFingerprint
}

// varFingerprint describes a single static variable.
type varFingerprint struct {
	// Type is the static type of the variable. It is nil when the type is
	// unknown (dynamic).
	Type reflect.Type
	// Unresolved is true when the static type cannot be determined. Such
	// variables are never treated as validated by Check.
	Unresolved bool
}

// funcFingerprint describes a single function name with all its signatures.
type funcFingerprint struct {
	// Signatures are the known function signatures registered for the name.
	Signatures []reflect.Type
	// FromEnv is true when the function originates from the environment (a
	// method of the env type). Compile-time functions registered via
	// expr.Function have FromEnv set to false.
	FromEnv bool
}

// modeFingerprint records the compilation mode.
type modeFingerprint struct {
	Optimize          bool
	Strict            bool
	ShortCircuit      bool
	Profile           bool
	Expect            reflect.Kind
	ExpectAny         bool
	DisableIfOperator bool
	MaxNodes          uint
}

// FingerprintOf returns the environment fingerprint for the given config.
// The fingerprint captures the static variable types, function signatures,
// patch options and compilation mode. It uses reflection only; host functions
// are never invoked.
func FingerprintOf(c *conf.Config) *EnvFingerprint {
	fp := &EnvFingerprint{
		variables: make(map[string]varFingerprint),
		functions: make(map[string]funcFingerprint),
	}
	if c == nil {
		return fp
	}

	// Static variables and methods of the environment.
	if c.Env.Type != nil || c.Env.Kind != 0 {
		for name, nt := range c.Env.All(&c.NtCache) {
			if nt.Method {
				if _, overridden := c.Functions[name]; overridden {
					// A compile-time function takes precedence over an env
					// method with the same name.
					continue
				}
				fp.functions[name] = funcFingerprint{
					Signatures: []reflect.Type{nt.Type},
					FromEnv:    true,
				}
				continue
			}
			if nt.Type != nil && nt.Type.Kind() == reflect.Func {
				// A callable value (map func value or struct func field) is
				// recorded as a function signature.
				fp.functions[name] = funcFingerprint{
					Signatures: []reflect.Type{nt.Type},
					FromEnv:    true,
				}
				continue
			}
			fp.variables[name] = varFingerprint{
				Type: nt.Type,
				// A variable is unresolved when its static type is unknown or
				// dynamic (an interface type). Such types are never treated
				// as validated.
				Unresolved: nt.Type == nil || nt.Kind == reflect.Interface,
			}
		}
	}

	// Compile-time functions registered via expr.Function.
	for name, fn := range c.Functions {
		sigs := make([]reflect.Type, 0, len(fn.Types))
		for _, t := range fn.Types {
			sigs = append(sigs, t)
		}
		fp.functions[name] = funcFingerprint{Signatures: sigs}
	}

	// Patch options (visitors).
	for _, v := range c.Visitors {
		fp.patches = append(fp.patches, patchID(v))
	}

	// Compilation mode.
	fp.mode = modeFingerprint{
		Optimize:          c.Optimize,
		Strict:            c.Strict,
		ShortCircuit:      c.ShortCircuit,
		Profile:           c.Profile,
		Expect:            c.Expect,
		ExpectAny:         c.ExpectAny,
		DisableIfOperator: c.DisableIfOperator,
		MaxNodes:          c.MaxNodes,
	}

	return fp
}

// Check verifies that the fingerprint is compatible with the other one.
// It uses reflection only; no host functions are invoked. An error is
// returned when a static variable changed its type, a function changed its
// signature, a patch option changed, the compilation mode changed, or a
// variable has an unresolved dynamic type.
func (fp *EnvFingerprint) Check(other *EnvFingerprint) error {
	if fp == nil {
		return fmt.Errorf("program environment check: the program was compiled without an environment fingerprint")
	}
	if other == nil {
		return fmt.Errorf("program environment check: the runtime environment fingerprint is nil")
	}

	// Variables.
	for name, va := range fp.variables {
		vb, ok := other.variables[name]
		if !ok {
			return fmt.Errorf("program environment check: variable %q is missing in the runtime environment", name)
		}
		if va.Unresolved {
			return fmt.Errorf("program environment check: variable %q has an unresolved dynamic type in the compiled program; its type cannot be validated", name)
		}
		if vb.Unresolved {
			return fmt.Errorf("program environment check: variable %q has an unresolved dynamic type in the runtime environment; its type cannot be validated", name)
		}
		if va.Type != vb.Type {
			return fmt.Errorf("program environment check: variable %q changed type from %s to %s", name, va.Type, vb.Type)
		}
	}

	// Functions.
	for name, fa := range fp.functions {
		fb, ok := other.functions[name]
		if !ok {
			if fa.FromEnv {
				return fmt.Errorf("program environment check: function %q is missing in the runtime environment", name)
			}
			// A compile-time function (expr.Function) is not part of the
			// runtime environment; nothing to check.
			continue
		}
		if !sameSignatures(fa.Signatures, fb.Signatures) {
			return fmt.Errorf("program environment check: function %q changed its signature", name)
		}
	}

	// Patch options.
	if !equalStrings(fp.patches, other.patches) {
		return fmt.Errorf("program environment check: patch options differ (compiled with %s, runtime with %s)",
			formatPatches(fp.patches), formatPatches(other.patches))
	}

	// Compilation mode.
	if fp.mode != other.mode {
		return fmt.Errorf("program environment check: compilation mode differs (compiled with %s, runtime with %s)",
			fp.mode, other.mode)
	}

	return nil
}

// sameSignatures reports whether the two signature sets are equal.
func sameSignatures(a, b []reflect.Type) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[reflect.Type]int, len(a))
	for _, t := range a {
		seen[t]++
	}
	for _, t := range b {
		seen[t]--
		if seen[t] < 0 {
			return false
		}
	}
	return true
}

// patchID returns a stable identifier for a patch visitor.
func patchID(v ast.Visitor) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.IsValid() && rv.Kind() == reflect.Struct {
		for _, field := range []string{"Operator", "Name", "Location"} {
			if f := rv.FieldByName(field); f.IsValid() {
				switch f.Kind() {
				case reflect.String:
					return fmt.Sprintf("%T(%s)", v, f.String())
				case reflect.Struct:
					return fmt.Sprintf("%T(%v)", v, f.Interface())
				}
			}
		}
	}
	return fmt.Sprintf("%T", v)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func formatPatches(patches []string) string {
	if len(patches) == 0 {
		return "none"
	}
	return strings.Join(patches, ", ")
}

// String returns a human-readable representation of the fingerprint.
func (fp *EnvFingerprint) String() string {
	if fp == nil {
		return "<nil>"
	}
	var b strings.Builder
	b.WriteString("EnvFingerprint{")
	first := true
	if len(fp.variables) > 0 {
		names := make([]string, 0, len(fp.variables))
		for n := range fp.variables {
			names = append(names, n)
		}
		sort.Strings(names)
		b.WriteString("variables:")
		for _, n := range names {
			if !first {
				b.WriteString(",")
			}
			first = false
			v := fp.variables[n]
			if v.Unresolved {
				fmt.Fprintf(&b, " %s=<unresolved>", n)
			} else {
				fmt.Fprintf(&b, " %s=%s", n, v.Type)
			}
		}
	}
	if len(fp.functions) > 0 {
		names := make([]string, 0, len(fp.functions))
		for n := range fp.functions {
			names = append(names, n)
		}
		sort.Strings(names)
		b.WriteString("; functions:")
		first = true
		for _, n := range names {
			if !first {
				b.WriteString(",")
			}
			first = false
			fmt.Fprintf(&b, " %s=%v", n, fp.functions[n].Signatures)
		}
	}
	if len(fp.patches) > 0 {
		fmt.Fprintf(&b, "; patches: %s", strings.Join(fp.patches, ", "))
	}
	fmt.Fprintf(&b, "; mode: %s", fp.mode)
	b.WriteString("}")
	return b.String()
}

// String returns a human-readable representation of the compilation mode.
func (m modeFingerprint) String() string {
	var parts []string
	if m.Optimize {
		parts = append(parts, "optimize")
	}
	if m.Strict {
		parts = append(parts, "strict")
	}
	if m.ShortCircuit {
		parts = append(parts, "short-circuit")
	}
	if m.Profile {
		parts = append(parts, "profile")
	}
	if m.Expect != reflect.Invalid {
		parts = append(parts, "expect="+m.Expect.String())
	}
	if m.ExpectAny {
		parts = append(parts, "expect-any")
	}
	if m.DisableIfOperator {
		parts = append(parts, "disable-if-operator")
	}
	if m.MaxNodes != 0 {
		parts = append(parts, fmt.Sprintf("max-nodes=%d", m.MaxNodes))
	}
	if len(parts) == 0 {
		return "default"
	}
	return strings.Join(parts, ",")
}
