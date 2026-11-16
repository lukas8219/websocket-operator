package diff_test

import (
	"github.com/hashicorp/go-set/v3"
	"lukas8219/websocket-operator/internal/diff"
	"testing"
)

func FuzzDifferenceModel(f *testing.F) {
	f.Add([]byte{1, 2, 3}, []byte{2, 3, 4}, []byte{1, 2, 3})
	f.Add([]byte{}, []byte{}, []byte{})
	f.Fuzz(func(t *testing.T, initial, add, remove []byte) {
		if len(initial)+len(add)+len(remove) > 4096 {
			t.Skip()
		}
		before := map[byte]bool{}
		want := map[byte]bool{}
		for _, v := range initial {
			before[v] = true
			want[v] = true
		}
		for _, v := range remove {
			delete(want, v)
		}
		for _, v := range add {
			want[v] = true
		}
		state := set.From(initial)
		result := diff.Difference(state, add, remove)
		if state.Size() != len(want) {
			t.Fatalf("size: got %v want %v", state, want)
		}
		for v := range want {
			if !state.Contains(v) {
				t.Fatalf("missing %d", v)
			}
		}
		added, removed := map[byte]bool{}, map[byte]bool{}
		for v := range want {
			if !before[v] {
				added[v] = true
			}
		}
		for v := range before {
			if !want[v] {
				removed[v] = true
			}
		}
		if len(result.Added) != len(added) || len(result.Removed) != len(removed) {
			t.Fatalf("wrong delta: %+v", result)
		}
		for _, v := range result.Added {
			if !added[v] {
				t.Fatalf("unexpected addition %d", v)
			}
		}
		for _, v := range result.Removed {
			if !removed[v] {
				t.Fatalf("unexpected removal %d", v)
			}
		}
		replay := diff.Difference(state, add, remove)
		if len(replay.Added) != 0 || len(replay.Removed) != 0 {
			t.Fatalf("non-idempotent update: %+v", replay)
		}
	})
}
