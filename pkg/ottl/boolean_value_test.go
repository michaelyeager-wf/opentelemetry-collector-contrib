// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ottl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottltest"
)

// valueFor is a test helper to eliminate a lot of tedium in writing tests of Comparisons.
func valueFor(x any) Value {
	val := Value{}
	switch v := x.(type) {
	case []byte:
		var b Bytes = v
		val.Bytes = &b
	case string:
		switch {
		case v == "NAME":
			// if the string is NAME construct a path of "name".
			val.Path = &Path{
				Fields: []Field{
					{
						Name: "name",
					},
				},
			}
		case strings.Contains(v, "ENUM"):
			// if the string contains ENUM construct an EnumSymbol from it.
			val.Enum = (*EnumSymbol)(ottltest.Strp(v))
		default:
			val.String = ottltest.Strp(v)
		}
	case float64:
		val.Float = ottltest.Floatp(v)
	case *float64:
		val.Float = v
	case int:
		val.Int = ottltest.Intp(int64(v))
	case *int64:
		val.Int = v
	case bool:
		val.Bool = Booleanp(Boolean(v))
	case nil:
		var n IsNil = true
		val.IsNil = &n
	default:
		panic("test error!")
	}
	return val
}

// comparison is a test helper that constructs a Comparison object using valueFor
func comparison(left any, right any, op string) *Comparison {
	return &Comparison{
		Left:  valueFor(left),
		Right: valueFor(right),
		Op:    compareOpTable[op],
	}
}

func Test_newComparisonEvaluator(t *testing.T) {
	p := NewParser(
		defaultFunctionsForTests(),
		testParsePath,
		testParseEnum,
		componenttest.NewNopTelemetrySettings(),
	)

	var tests = []struct {
		name string
		l    any
		r    any
		op   string
		item string
		want bool
	}{
		{name: "literals match", l: "hello", r: "hello", op: "==", want: true},
		{name: "literals don't match", l: "hello", r: "goodbye", op: "!=", want: true},
		{name: "path expression matches", l: "NAME", r: "bear", op: "==", item: "bear", want: true},
		{name: "path expression not matches", l: "NAME", r: "cat", op: "!=", item: "bear", want: true},
		{name: "compare Enum to int", l: "TEST_ENUM", r: 0, op: "==", want: true},
		{name: "compare int to Enum", l: 2, r: "TEST_ENUM_TWO", op: "==", want: true},
		{name: "2 > Enum 0", l: 2, r: "TEST_ENUM", op: ">", want: true},
		{name: "not 2 < Enum 0", l: 2, r: "TEST_ENUM", op: "<"},
		{name: "not 6 == 3.14", l: 6, r: 3.14, op: "=="},
		{name: "6 != 3.14", l: 6, r: 3.14, op: "!=", want: true},
		{name: "6 > 3.14", l: 6, r: 3.14, op: ">", want: true},
		{name: "6 >= 3.14", l: 6, r: 3.14, op: ">=", want: true},
		{name: "not 6 < 3.14", l: 6, r: 3.14, op: "<"},
		{name: "not 6 <= 3.14", l: 6, r: 3.14, op: "<="},
		{name: "'foo' > 'bar'", l: "foo", r: "bar", op: ">", want: true},
		{name: "'foo' > bear", l: "foo", r: "NAME", op: ">", item: "bear", want: true},
		{name: "true > false", l: true, r: false, op: ">", want: true},
		{name: "not true > 0", l: true, r: 0, op: ">"},
		{name: "not 'true' == true", l: "true", r: true, op: "=="},
		{name: "[]byte('a') < []byte('b')", l: []byte("a"), r: []byte("b"), op: "<", want: true},
		{name: "nil == nil", op: "==", want: true},
		{name: "nil == []byte(nil)", r: []byte(nil), op: "==", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := comparison(tt.l, tt.r, tt.op)
			evaluate, err := p.newComparisonEvaluator(comp)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, evaluate(tt.item))
		})
	}
}

func Test_newConditionEvaluator_invalid(t *testing.T) {
	p := NewParser(
		defaultFunctionsForTests(),
		testParsePath,
		testParseEnum,
		component.TelemetrySettings{},
	)

	tests := []struct {
		name       string
		comparison *Comparison
	}{
		{
			name: "unknown Path",
			comparison: &Comparison{
				Left: Value{
					Enum: (*EnumSymbol)(ottltest.Strp("SYMBOL_NOT_FOUND")),
				},
				Op: EQ,
				Right: Value{
					String: ottltest.Strp("trash"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.newComparisonEvaluator(tt.comparison)
			assert.Error(t, err)
		})
	}
}

func Test_newBooleanExpressionEvaluator(t *testing.T) {
	p := NewParser(
		defaultFunctionsForTests(),
		testParsePath,
		testParseEnum,
		component.TelemetrySettings{},
	)

	tests := []struct {
		name string
		want bool
		expr *BooleanExpression
	}{
		{"a", false,
			&BooleanExpression{
				Left: &Term{
					Left: &BooleanValue{
						ConstExpr: Booleanp(true),
					},
					Right: []*OpAndBooleanValue{
						{
							Operator: "and",
							Value: &BooleanValue{
								ConstExpr: Booleanp(false),
							},
						},
					},
				},
			},
		},
		{"b", true,
			&BooleanExpression{
				Left: &Term{
					Left: &BooleanValue{
						ConstExpr: Booleanp(true),
					},
					Right: []*OpAndBooleanValue{
						{
							Operator: "and",
							Value: &BooleanValue{
								ConstExpr: Booleanp(true),
							},
						},
					},
				},
			},
		},
		{"c", false,
			&BooleanExpression{
				Left: &Term{
					Left: &BooleanValue{
						ConstExpr: Booleanp(true),
					},
					Right: []*OpAndBooleanValue{
						{
							Operator: "and",
							Value: &BooleanValue{
								ConstExpr: Booleanp(true),
							},
						},
						{
							Operator: "and",
							Value: &BooleanValue{
								ConstExpr: Booleanp(false),
							},
						},
					},
				},
			},
		},
		{"d", true,
			&BooleanExpression{
				Left: &Term{
					Left: &BooleanValue{
						ConstExpr: Booleanp(true),
					},
				},
				Right: []*OpOrTerm{
					{
						Operator: "or",
						Term: &Term{
							Left: &BooleanValue{
								ConstExpr: Booleanp(false),
							},
						},
					},
				},
			},
		},
		{"e", true,
			&BooleanExpression{
				Left: &Term{
					Left: &BooleanValue{
						ConstExpr: Booleanp(false),
					},
				},
				Right: []*OpOrTerm{
					{
						Operator: "or",
						Term: &Term{
							Left: &BooleanValue{
								ConstExpr: Booleanp(true),
							},
						},
					},
				},
			},
		},
		{"f", false,
			&BooleanExpression{
				Left: &Term{
					Left: &BooleanValue{
						ConstExpr: Booleanp(false),
					},
				},
				Right: []*OpOrTerm{
					{
						Operator: "or",
						Term: &Term{
							Left: &BooleanValue{
								ConstExpr: Booleanp(false),
							},
						},
					},
				},
			},
		},
		{"g", true,
			&BooleanExpression{
				Left: &Term{
					Left: &BooleanValue{
						ConstExpr: Booleanp(false),
					},
					Right: []*OpAndBooleanValue{
						{
							Operator: "and",
							Value: &BooleanValue{
								ConstExpr: Booleanp(false),
							},
						},
					},
				},
				Right: []*OpOrTerm{
					{
						Operator: "or",
						Term: &Term{
							Left: &BooleanValue{
								ConstExpr: Booleanp(true),
							},
						},
					},
				},
			},
		},
		{"h", true,
			&BooleanExpression{
				Left: &Term{
					Left: &BooleanValue{
						ConstExpr: Booleanp(true),
					},
					Right: []*OpAndBooleanValue{
						{
							Operator: "and",
							Value: &BooleanValue{
								SubExpr: &BooleanExpression{
									Left: &Term{
										Left: &BooleanValue{
											ConstExpr: Booleanp(true),
										},
									},
									Right: []*OpOrTerm{
										{
											Operator: "or",
											Term: &Term{
												Left: &BooleanValue{
													ConstExpr: Booleanp(false),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluate, err := p.newBooleanExpressionEvaluator(tt.expr)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, evaluate(nil))
		})
	}
}
