package vm

import (
	"fmt"

	"github.com/expr-lang/expr/file"
)

// ContextCanceledError is returned when the VM's context is canceled during execution.
//
// The error carries the opcode (instruction) at which cancellation was detected and
// the source position derived from the compile-time location mapping. Cancellation is
// only ever observed at controlled instruction boundaries, so a host call that has
// already started is not interrupted.
type ContextCanceledError struct {
	FileError *file.Error

	// Op is the opcode of the instruction at which the context was canceled.
	Op Opcode
}

func (e *ContextCanceledError) Error() string {
	if e.FileError.Snippet == "" {
		return fmt.Sprintf("%s at %s", e.FileError.Message, e.Op)
	}
	return fmt.Sprintf("%s at %s (%d:%d)%s", e.FileError.Message, e.Op, e.FileError.Line, e.FileError.Column+1, e.FileError.Snippet)
}

func (e *ContextCanceledError) Unwrap() error {
	return e.FileError.Unwrap()
}

// contextCanceled is the sentinel panic value used internally to unwind the
// instruction loop when the context is canceled. It is converted to a
// ContextCanceledError by the RunWithContext method's recover handler.
type contextCanceled struct {
	op  Opcode
	err error
}

func (e *contextCanceled) Error() string {
	return fmt.Sprintf("context canceled at %s: %v", e.op, e.err)
}

func (e *contextCanceled) Unwrap() error {
	return e.err
}

// ensure contextCanceled satisfies the error interface at compile time.
var _ error = (*contextCanceled)(nil)
