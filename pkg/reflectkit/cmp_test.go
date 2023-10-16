package reflectkit_test

import (
	"go.llib.dev/frameless/pkg/reflectkit"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func ExampleEqual() {
	reflectkit.Equal("a", "a") // true
	reflectkit.Equal("a", "b") // false
}

func TestEqual(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	tt := []struct {
		desc    string
		v1, v2  any
		isEqual bool
	}{
		{
			desc:    "equal integers",
			v1:      1,
			v2:      1,
			isEqual: true,
		},
		{
			desc:    "different integers",
			v1:      1,
			v2:      2,
			isEqual: false,
		},
		{
			desc:    "equal strings",
			v1:      "test",
			v2:      "test",
			isEqual: true,
		},
		{
			desc:    "different strings",
			v1:      "test",
			v2:      "test1",
			isEqual: false,
		},
		{
			desc:    "equal slices",
			v1:      []int{1, 2, 3},
			v2:      []int{1, 2, 3},
			isEqual: true,
		},
		{
			desc:    "different slices",
			v1:      []int{1, 2, 3},
			v2:      []int{1, 2, 4},
			isEqual: false,
		},
		{
			desc:    "equal arrays",
			v1:      [3]int{1, 2, 3},
			v2:      [3]int{1, 2, 3},
			isEqual: true,
		},
		{
			desc:    "different arrays",
			v1:      [3]int{1, 2, 3},
			v2:      [3]int{1, 2, 4},
			isEqual: false,
		},
		{
			desc:    "equal maps",
			v1:      map[string]int{"one": 1, "two": 2},
			v2:      map[string]int{"one": 1, "two": 2},
			isEqual: true,
		},
		{
			desc:    "different maps",
			v1:      map[string]int{"one": 1, "two": 2},
			v2:      map[string]int{"one": 1, "two": 3},
			isEqual: false,
		},
		{
			desc:    "equal structs",
			v1:      TestStruct{Field1: 1, Field2: "test"},
			v2:      TestStruct{Field1: 1, Field2: "test"},
			isEqual: true,
		},
		{
			desc:    "different structs",
			v1:      TestStruct{Field1: 1, Field2: "test"},
			v2:      TestStruct{Field1: 2, Field2: "test"},
			isEqual: false,
		},
		{
			desc: "different structs with equality support - equal",
			v1: TestStructEquatable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructEquatable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with equality support - not equal",
			v1: TestStructEquatable{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructEquatable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
		{
			desc: "different structs with equality support (ptr receiver) - equal",
			v1: TestStructEquatableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructEquatableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with equality support (ptr receiver) - not equal",
			v1: TestStructEquatableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructEquatableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
		{
			desc: "different structs with equality support (IsEqual) - equal",
			v1: TestStructEquatableWithIsEqual{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructEquatableWithIsEqual{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with equality support (IsEqual) - not equal",
			v1: TestStructEquatableWithIsEqual{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructEquatableWithIsEqual{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
		{
			desc: "different structs with comparable support - equal",
			v1: TestStructComparable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructComparable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with comparable support - not equal",
			v1: TestStructComparable{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructComparable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
		{
			desc: "different structs with comparable support (ptr receiver) - equal",
			v1: TestStructComparableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructComparableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with comparable support (ptr receiver) - not equal",
			v1: TestStructComparablePointers{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructComparablePointers{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
	}
	for _, tc := range tt {
		t.Run(tc.desc, func(t *testing.T) {
			got := reflectkit.Equal(tc.v1, tc.v2)
			if got != tc.isEqual {
				t.Errorf("DeepEqual() = %v, want %v", got, tc.isEqual)
			}
		})
	}
}

type TestStruct struct {
	Field1 int
	Field2 string
}

type TestStructEquatable struct {
	Irrelevant int
	Relevant   int
}

func (es TestStructEquatable) Equal(oth TestStructEquatable) bool {
	return es.Relevant == oth.Relevant
}

type TestStructEquatableOnPtr struct {
	Irrelevant int
	Relevant   int
}

func (es *TestStructEquatableOnPtr) Equal(oth TestStructEquatableOnPtr) bool {
	return es.Relevant == oth.Relevant
}

type TestStructEquatableWithIsEqual struct {
	Irrelevant int
	Relevant   int
}

func (es TestStructEquatableWithIsEqual) IsEqual(oth TestStructEquatableWithIsEqual) bool {
	return es.Relevant == oth.Relevant
}

func cmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a == b:
		return 0
	case a > b:
		return 1
	default:
		panic("unknown Cmp case")
	}
}

type TestStructComparable struct {
	Irrelevant int
	Relevant   int
}

func (es TestStructComparable) Cmp(v TestStructComparable) int {
	return cmp(es.Relevant, v.Relevant)
}

type TestStructComparableOnPtr struct {
	Irrelevant int
	Relevant   int
}

func (es *TestStructComparableOnPtr) Cmp(v TestStructComparableOnPtr) int {
	return cmp(es.Relevant, v.Relevant)
}

type TestStructComparablePointers struct {
	Irrelevant int
	Relevant   int
}

func (es *TestStructComparablePointers) Cmp(v *TestStructComparablePointers) int {
	return cmp(es.Relevant, v.Relevant)
}
