package vm

type Opcode byte

var opcodeNames = map[Opcode]string{
	OpInvalid:         "OpInvalid",
	OpPush:            "OpPush",
	OpInt:             "OpInt",
	OpPop:             "OpPop",
	OpStore:           "OpStore",
	OpLoadVar:         "OpLoadVar",
	OpLoadConst:       "OpLoadConst",
	OpLoadField:       "OpLoadField",
	OpLoadFast:        "OpLoadFast",
	OpLoadMethod:      "OpLoadMethod",
	OpLoadFunc:        "OpLoadFunc",
	OpLoadEnv:         "OpLoadEnv",
	OpFetch:           "OpFetch",
	OpFetchField:      "OpFetchField",
	OpMethod:          "OpMethod",
	OpTrue:            "OpTrue",
	OpFalse:           "OpFalse",
	OpNil:             "OpNil",
	OpNegate:          "OpNegate",
	OpNot:             "OpNot",
	OpEqual:           "OpEqual",
	OpEqualInt:        "OpEqualInt",
	OpEqualString:     "OpEqualString",
	OpJump:            "OpJump",
	OpJumpIfTrue:      "OpJumpIfTrue",
	OpJumpIfFalse:     "OpJumpIfFalse",
	OpJumpIfNil:       "OpJumpIfNil",
	OpJumpIfNotNil:    "OpJumpIfNotNil",
	OpJumpIfEnd:       "OpJumpIfEnd",
	OpJumpBackward:    "OpJumpBackward",
	OpIn:              "OpIn",
	OpLess:            "OpLess",
	OpMore:            "OpMore",
	OpLessOrEqual:     "OpLessOrEqual",
	OpMoreOrEqual:     "OpMoreOrEqual",
	OpAdd:             "OpAdd",
	OpSubtract:        "OpSubtract",
	OpMultiply:        "OpMultiply",
	OpDivide:          "OpDivide",
	OpModulo:          "OpModulo",
	OpExponent:        "OpExponent",
	OpRange:           "OpRange",
	OpMatches:         "OpMatches",
	OpMatchesConst:    "OpMatchesConst",
	OpContains:        "OpContains",
	OpStartsWith:      "OpStartsWith",
	OpEndsWith:        "OpEndsWith",
	OpSlice:           "OpSlice",
	OpCall:            "OpCall",
	OpCall0:           "OpCall0",
	OpCall1:           "OpCall1",
	OpCall2:           "OpCall2",
	OpCall3:           "OpCall3",
	OpCallN:           "OpCallN",
	OpCallFast:        "OpCallFast",
	OpCallSafe:        "OpCallSafe",
	OpCallTyped:       "OpCallTyped",
	OpCallBuiltin1:    "OpCallBuiltin1",
	OpArray:           "OpArray",
	OpMap:             "OpMap",
	OpLen:             "OpLen",
	OpCast:            "OpCast",
	OpDeref:           "OpDeref",
	OpIncrementIndex:  "OpIncrementIndex",
	OpDecrementIndex:  "OpDecrementIndex",
	OpIncrementCount:  "OpIncrementCount",
	OpGetIndex:        "OpGetIndex",
	OpGetCount:        "OpGetCount",
	OpGetLen:          "OpGetLen",
	OpGetAcc:          "OpGetAcc",
	OpSetAcc:          "OpSetAcc",
	OpSetIndex:        "OpSetIndex",
	OpPointer:         "OpPointer",
	OpThrow:           "OpThrow",
	OpCreate:          "OpCreate",
	OpGroupBy:         "OpGroupBy",
	OpSortBy:          "OpSortBy",
	OpSort:            "OpSort",
	OpProfileStart:    "OpProfileStart",
	OpProfileEnd:      "OpProfileEnd",
	OpBegin:           "OpBegin",
	OpAnd:             "OpAnd",
	OpOr:              "OpOr",
	OpEnd:             "OpEnd",
}

// String returns the name of the opcode.
func (op Opcode) String() string {
	if name, ok := opcodeNames[op]; ok {
		return name
	}
	return "OpUnknown"
}

const (
	OpInvalid Opcode = iota
	OpPush
	OpInt
	OpPop
	OpStore
	OpLoadVar
	OpLoadConst
	OpLoadField
	OpLoadFast
	OpLoadMethod
	OpLoadFunc
	OpLoadEnv
	OpFetch
	OpFetchField
	OpMethod
	OpTrue
	OpFalse
	OpNil
	OpNegate
	OpNot
	OpEqual
	OpEqualInt
	OpEqualString
	OpJump
	OpJumpIfTrue
	OpJumpIfFalse
	OpJumpIfNil
	OpJumpIfNotNil
	OpJumpIfEnd
	OpJumpBackward
	OpIn
	OpLess
	OpMore
	OpLessOrEqual
	OpMoreOrEqual
	OpAdd
	OpSubtract
	OpMultiply
	OpDivide
	OpModulo
	OpExponent
	OpRange
	OpMatches
	OpMatchesConst
	OpContains
	OpStartsWith
	OpEndsWith
	OpSlice
	OpCall
	OpCall0
	OpCall1
	OpCall2
	OpCall3
	OpCallN
	OpCallFast
	OpCallSafe
	OpCallTyped
	OpCallBuiltin1
	OpArray
	OpMap
	OpLen
	OpCast
	OpDeref
	OpIncrementIndex
	OpDecrementIndex
	OpIncrementCount
	OpGetIndex
	OpGetCount
	OpGetLen
	OpGetAcc
	OpSetAcc
	OpSetIndex
	OpPointer
	OpThrow
	OpCreate
	OpGroupBy
	OpSortBy
	OpSort
	OpProfileStart
	OpProfileEnd
	OpBegin
	OpAnd
	OpOr
	OpEnd // This opcode must be at the end of this list.
)
