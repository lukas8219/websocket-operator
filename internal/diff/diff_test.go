package diff_test

import (
	"github.com/hashicorp/go-set/v3"
	"lukas8219/websocket-operator/internal/diff"
	"testing"
)

func TestDifferenceOutputOnAtomicUpsert(t *testing.T) {
	current := set.From([]int{1, 2, 3})
	output := diff.Difference(current, []int{4, 3, 2, 5}, []int{3, 1})
	if !set.From(output.Added).EqualSlice([]int{4, 5}) {
		t.Errorf("added = %v, want [4 5]", output.Added)
	}
	// An entry present in the new snapshot remains even when it was in the old snapshot.
	if !set.From(output.Removed).EqualSlice([]int{1}) {
		t.Errorf("removed = %v, want [1]", output.Removed)
	}
	if !current.EqualSlice([]int{2, 3, 4, 5}) {
		t.Errorf("state = %v, want [2 3 4 5]", current)
	}
}

func TestDifferenceRemovesFinalHost(t *testing.T) {
	current := set.From([]string{"host"})
	output := diff.Difference(current, nil, []string{"host"})
	if current.Size() != 0 || len(output.Added) != 0 || !set.From(output.Removed).EqualSlice([]string{"host"}) {
		t.Fatalf("removing final host: state=%v, diff=%+v", current, output)
	}
}
