package simple

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONEncode(t *testing.T) {
	require.Equal(t, `{"alpha":false,"bravo":2.13,"charlie":"delta","echo":["hello","world","!"]}`, Struct{
		"alpha":   Bool(false),
		"bravo":   Number(2.13),
		"charlie": String("delta"),
		"echo":    Array{String("hello"), String("world"), String("!")},
	}.String())
}

func TestJSONDecode(t *testing.T) {
	t.Run("FromJSON", func(t *testing.T) {
		type tc struct {
			input  string
			output Value
		}
		for _, testCase := range []tc{
			{input: `{"alpha":["beta", 1]}`, output: Struct{
				"alpha": Array{
					String("beta"),
					Number(1),
				},
			}},
			{input: `3.1415`, output: Number(3.1415)},
			{
				input: `[{"alpha":[{"bravo":false},{"charlie":null,"delta":"echo","foxtrot":3.140002}]}]`,
				output: Array{
					Struct{
						"alpha": Array{
							Struct{"bravo": Bool(false)},
							Struct{"charlie": nil, "delta": String("echo"), "foxtrot": Number(3.140002)},
						},
					},
				},
			},
		} {
			t.Run(testCase.input, func(t *testing.T) {
				v, err := FromJSON(json.RawMessage(testCase.input))
				require.NoError(t, err)
				require.Equal(t, testCase.output, v)
			})
		}
	})
	t.Run("Unmarshal", func(t *testing.T) {
		type testCase struct {
			target   func() any
			input    string
			expected Value
		}
		for idx, tc := range []testCase{
			{
				target:   func() any { return new(Struct) },
				input:    `{"foo":"bar"}`,
				expected: &Struct{"foo": String("bar")},
			},
			{
				target:   func() any { return new(Array) },
				input:    `[1, 2, 3]`,
				expected: &Array{Number(1), Number(2), Number(3)},
			},
		} {
			t.Run(strconv.Itoa(idx), func(t *testing.T) {
				dest := tc.target()
				err := json.Unmarshal([]byte(tc.input), dest)
				require.NoError(t, err)
				require.Equal(t, tc.expected, dest)
			})
		}
	})
}

func TestFromValue(t *testing.T) {
	type testCase struct {
		name        string
		input       func() any
		expectError func(*testing.T, error)
		output      Value
	}

	for _, tc := range []testCase{
		{
			name:  "from nil",
			input: func() any { return nil },
			// expectError: ni,
			output: nil,
		},
		{
			name: "nil pointer",
			input: func() any {
				var i *int
				return i
			},
			output: nil,
		},
		{
			name: "zero field struct",
			input: func() any {
				return struct{}{}
			},
			output: Struct{},
		},
		{
			name: "typed interface, concrete value",
			input: func() any {
				type a struct {
					B error
					C int
				}
				return a{B: errors.New("test?"), C: 1}
			},
			output: Struct{
				"B": Struct{},
				"C": Number(1),
			},
		},
		{
			name: "recursive map in struct",
			input: func() any {
				type a struct {
					M map[string]a
				}

				return a{
					M: map[string]a{"Nothing": {}},
				}
			},
			output: Struct{
				"M": Struct{
					"Nothing": Struct{
						"M": Struct{},
					},
				},
			},
		},
		{
			name: "non-stringable key in map",
			input: func() any {
				type mk [3]int
				type a struct {
					M map[mk]string
				}
				return map[int]any{
					5: a{
						M: map[mk]string{
							{2, 3, 4}: "cool?",
						},
					},
					10: false,
				}
			},
			expectError: func(t *testing.T, err error) {
				require.Equal(t, err.Error(), `cannot convert value at .5.M: map key with array type "simple.mk" cannot be stringified`)
			},
		},
		{
			name: "non-simple value in array",
			input: func() any {
				type complexArray [1]chan int
				return map[string]any{
					"p": complexArray{make(chan int, 1)},
				}
			},
			expectError: func(t *testing.T, err error) {
				require.Equal(t, err.Error(), `cannot convert value at .p[0]: cannot convert value of kind chan to simple value`)
			},
		},
		{
			name: "other scalar types okay",
			input: func() any {
				type wildArray [3]any
				return map[string]any{
					"stuff": wildArray{false, math.Pi, "hello"},
				}
			},
			output: Struct{
				"stuff": Array{
					Bool(false),
					Number(math.Pi),
					String("hello"),
				},
			},
		},
		{
			name: "non builtin scalar values",
			input: func() any {
				type mySpecialBool bool
				type mySpecialString string
				type mySpecialNumber uint16
				type mySpecialOtherNumber uintptr
				return map[mySpecialNumber]any{
					62: mySpecialBool(true),
					63: mySpecialString("what is even happening?"),
					64: mySpecialOtherNumber(123),
				}
			},
			output: Struct{
				"62": Bool(true),
				"63": String("what is even happening?"),
				"64": Number(123),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FromValue(tc.input())
			if tc.expectError != nil {
				require.Error(t, err)
				tc.expectError(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.output, got)
			}
		})
	}
}
