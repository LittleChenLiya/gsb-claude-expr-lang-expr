package expr

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/builtin"
	"github.com/expr-lang/expr/checker/nature"
	"github.com/expr-lang/expr/conf"
	"github.com/expr-lang/expr/vm"
)

// EnvCompatibilityError is returned by Check and Run when a compiled program
// is about to be executed with an environment that is incompatible with the
// environment fingerprint recorded at compile time.
type EnvCompatibilityError struct {
	Problems []string
}

// Error implements the error interface.
func (e *EnvCompatibilityError) Error() string {
	return "expr: program is not compatible with the given environment: " +
		strings.Join(e.Problems, "; ")
}

// Check verifies that the given environment is compatible with the
// environment fingerprint recorded in the program at compile time. It
// compares static variable types and function signatures only; dynamic
// values are never part of the fingerprint. Check performs a pure
// pre-flight validation: it uses reflection over types and never executes
// any host function. Programs compiled without a fingerprint (e.g. via
// expr.Eval) are always accepted.
//
// Only names present in the given environment are verified: a missing name
// is a runtime concern (e.g. optional chaining or short-circuit evaluation
// may never touch it), not a type incompatibility.
func Check(program *vm.Program, env any) error {
	if program == nil {
		return nil // vm.Run reports nil programs.
	}
	fp := program.EnvFingerprint()
	if fp == nil {
		return nil
	}
	members := envSignature(env)
	var problems []string
	for _, name := range sortedKeys(fp.Variables) {
		want := fp.Variables[name]
		if want == "" {
			// The type was unresolved (dynamic) at compile time; it is not
			// statically verified and is left to the runtime.
			continue
		}
		m, ok := members[name]
		if !ok {
			continue // Absent names are handled by the runtime.
		}
		if m.isFunc {
			problems = append(problems, fmt.Sprintf("variable %q of type %s is a function in the given environment", name, want))
			continue
		}
		if m.desc != want && !typesCompatible(fp.VariableTypes[name], m.typ) {
			problems = append(problems, fmt.Sprintf("variable %q has type %s, but the program was compiled against type %s", name, describeType(m.desc), want))
		}
	}
	for _, name := range sortedKeys(fp.Functions) {
		want := fp.Functions[name]
		m, ok := members[name]
		if !ok {
			continue // Absent names are handled by the runtime.
		}
		if !m.isFunc {
			problems = append(problems, fmt.Sprintf("function %q (%s) is a variable in the given environment", name, want))
			continue
		}
		if m.desc != want {
			problems = append(problems, fmt.Sprintf("function %q has signature %s, but the program was compiled against %s", name, m.desc, want))
		}
	}
	if len(problems) > 0 {
		return &EnvCompatibilityError{Problems: problems}
	}
	return nil
}

func describeType(typ string) string {
	if typ == "" {
		return "a dynamic (unresolved) type"
	}
	return typ
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// buildEnvFingerprint records the static environment a program is compiled
// against: static types of referenced variables, signatures of referenced
// functions, applied patch options and the compile mode. Dynamic values are
// never recorded.
func buildEnvFingerprint(config *conf.Config, node ast.Node) *vm.EnvFingerprint {
	fp := &vm.EnvFingerprint{
		Variables:     make(map[string]string),
		VariableTypes: make(map[string]reflect.Type),
		Functions:     make(map[string]string),
	}
	for _, name := range referencedEnvNames(node) {
		if _, ok := config.Functions[name]; ok {
			continue // compile-time function, recorded in Options below.
		}
		if _, ok := config.Builtins[name]; ok {
			continue // builtin function, identical for every program.
		}
		nt, ok := config.Env.Get(&config.NtCache, name)
		if !ok {
			// Referenced, but unknown at compile time (e.g. with
			// AllowUndefinedVariables): a dynamic reference which is not
			// statically verified.
			fp.Variables[name] = ""
			continue
		}
		m := staticNature(&config.NtCache, nt)
		if m.isFunc {
			fp.Functions[name] = m.desc
		} else {
			fp.Variables[name] = m.desc
			if m.typ != nil {
				fp.VariableTypes[name] = m.typ
			}
		}
	}
	for _, v := range config.Visitors {
		fp.Options = append(fp.Options, fmt.Sprintf("patch:%T", v))
	}
	for _, name := range sortedFuncNames(config.Functions) {
		fp.Options = append(fp.Options, "function:"+name+":"+functionSignature(config.Functions[name]))
	}
	sort.Strings(fp.Options)

	mode := fmt.Sprintf("expect=%s;expectAny=%t;optimize=%t;strict=%t;shortCircuit=%t;disableIfOperator=%t;maxNodes=%d",
		config.Expect, config.ExpectAny, config.Optimize, config.Strict,
		config.ShortCircuit, config.DisableIfOperator, config.MaxNodes)
	if len(config.ConstFns) > 0 {
		names := make([]string, 0, len(config.ConstFns))
		for name := range config.ConstFns {
			names = append(names, name)
		}
		sort.Strings(names)
		mode += ";constFns=" + strings.Join(names, ",")
	}
	if len(config.Disabled) > 0 {
		names := make([]string, 0, len(config.Disabled))
		for name := range config.Disabled {
			names = append(names, name)
		}
		sort.Strings(names)
		mode += ";disabled=" + strings.Join(names, ",")
	}
	fp.Mode = mode
	return fp
}

func sortedFuncNames(table conf.FunctionsTable) []string {
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// envMember describes a single member of an environment: either a function
// signature or a variable type. An empty desc with a nil typ means the type
// is dynamic (unresolved) and not statically verified.
type envMember struct {
	isFunc bool
	desc   string
	typ    reflect.Type
}

// envSignature computes static variable types and function signatures of the
// given runtime environment. Unsupported environment kinds yield an empty
// signature, in which case nothing can be verified statically.
func envSignature(env any) (members map[string]envMember) {
	members = make(map[string]envMember)
	defer func() {
		// conf.EnvWithCache panics on unsupported environment kinds; such
		// environments carry no static information.
		_ = recover()
	}()
	cache := new(nature.Cache)
	nt := conf.EnvWithCache(cache, env)
	for name, n := range nt.All(cache) {
		members[name] = staticNature(cache, n)
	}
	return members
}

// staticNature classifies a nature as either a function signature or a
// variable type.
func staticNature(c *nature.Cache, nt nature.Nature) envMember {
	if nt.Type == nil {
		return envMember{} // nil or unknown value.
	}
	if nt.Kind == reflect.Func {
		return envMember{isFunc: true, desc: funcSignature(nt), typ: nt.Type}
	}
	if nt.IsUnknown(c) {
		return envMember{}
	}
	return envMember{desc: nt.Type.String(), typ: nt.Type}
}

// typesCompatible reports whether a runtime value of type got can stand in
// for a variable compiled against type want. Beyond identical types, two
// types are compatible when one satisfies the other as an interface, or when
// they share a common non-empty method set (e.g. two distinct implementations
// of context.Context).
func typesCompatible(want, got reflect.Type) bool {
	if want == nil || got == nil {
		return false
	}
	if want == got {
		return true
	}
	if want.Kind() == reflect.Interface && got.Implements(want) {
		return true
	}
	if got.Kind() == reflect.Interface && want.Implements(got) {
		return true
	}
	return sharesMethodSet(want, got)
}

// sharesMethodSet reports whether both types have at least one method and
// the smaller method set is a subset of the larger one.
func sharesMethodSet(a, b reflect.Type) bool {
	if a.NumMethod() == 0 || b.NumMethod() == 0 {
		return false
	}
	small, big := a, b
	if small.NumMethod() > big.NumMethod() {
		small, big = big, small
	}
	for i := 0; i < small.NumMethod(); i++ {
		m := small.Method(i)
		other, ok := big.MethodByName(m.Name)
		if !ok {
			return false
		}
		if methodSignature(small, m) != methodSignature(big, other) {
			return false
		}
	}
	return true
}

// methodSignature renders the signature of a method without its receiver.
func methodSignature(t reflect.Type, m reflect.Method) string {
	// reflect.Method.Type includes the receiver for concrete types, but not
	// for interface types.
	return m.Name + ":" + funcTypeSignature(m.Type, t.Kind() != reflect.Interface)
}

// funcSignature renders the signature of a function nature. The receiver of
// methods is dropped, so a method and a plain function with equal signatures
// are considered interchangeable.
func funcSignature(nt nature.Nature) string {
	return funcTypeSignature(nt.Type, nt.Method)
}

// funcTypeSignature renders a func type as "func(int) string". If
// dropReceiver is set, the first argument is omitted.
func funcTypeSignature(t reflect.Type, dropReceiver bool) string {
	if t == nil || t.Kind() != reflect.Func {
		return "func"
	}
	start := 0
	if dropReceiver && t.NumIn() > 0 {
		start = 1 // Drop the receiver.
	}
	var b strings.Builder
	b.WriteString("func(")
	for i := start; i < t.NumIn(); i++ {
		if i > start {
			b.WriteString(", ")
		}
		in := t.In(i)
		if t.IsVariadic() && i == t.NumIn()-1 {
			b.WriteString("..." + in.Elem().String())
		} else {
			b.WriteString(in.String())
		}
	}
	b.WriteString(")")
	switch t.NumOut() {
	case 0:
	case 1:
		b.WriteString(" " + t.Out(0).String())
	default:
		b.WriteString(" (")
		for i := 0; i < t.NumOut(); i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(t.Out(i).String())
		}
		b.WriteString(")")
	}
	return b.String()
}

// functionSignature renders the signature of a compile-time function defined
// via the expr.Function option.
func functionSignature(fn *builtin.Function) string {
	if len(fn.Types) > 0 {
		sigs := make([]string, 0, len(fn.Types))
		for _, t := range fn.Types {
			sigs = append(sigs, t.String())
		}
		return strings.Join(sigs, "|")
	}
	if fn.Func != nil {
		return reflect.TypeOf(fn.Func).String()
	}
	return "func"
}

// envRefCollector collects names of identifiers referenced by a compiled
// AST, excluding variables declared inside the expression itself.
type envRefCollector struct {
	declared map[string]struct{}
	names    map[string]struct{}
}

func (c *envRefCollector) Visit(node *ast.Node) {
	switch n := (*node).(type) {
	case *ast.VariableDeclaratorNode:
		c.declared[n.Name] = struct{}{}
	case *ast.IdentifierNode:
		c.names[n.Value] = struct{}{}
	}
}

// referencedEnvNames returns sorted names of environment identifiers the
// compiled program actually references.
func referencedEnvNames(node ast.Node) []string {
	c := &envRefCollector{
		declared: make(map[string]struct{}),
		names:    make(map[string]struct{}),
	}
	ast.Walk(&node, c)
	names := make([]string, 0, len(c.names))
	for name := range c.names {
		if _, ok := c.declared[name]; ok {
			continue
		}
		// Skip special identifiers such as $env, #, #acc, #index.
		if strings.HasPrefix(name, "$") || strings.HasPrefix(name, "#") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
