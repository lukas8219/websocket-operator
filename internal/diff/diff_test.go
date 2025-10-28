package diff_test

import (
	"lukas8219/websocket-operator/internal/diff"
	"testing"

	"github.com/hashicorp/go-set/v3"
)

func TestDifferenceOuputOnAtomicUpsert(t *testing.T) {
	initialState := set.New[int](10)
	initialState.InsertSlice([]int{1, 2, 3})
	output := diff.Difference(initialState, []int{4, 3, 2, 5}, []int{3, 1})
	if !set.From(output.Added).EqualSlice([]int{4, 5}) {
		t.Error("Output expected to have added int 4,5")
	}

	if !set.From(output.Removed).EqualSlice([]int{3, 1}) {
		t.Error("Output expected to have remove 3,1")
	}

	if !initialState.EqualSlice([]int{2, 4, 5}) {
		t.Error("Final state expected is 2,4,5", initialState.String())
	}
}
