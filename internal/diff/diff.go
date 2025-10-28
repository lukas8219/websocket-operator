package diff

import "github.com/hashicorp/go-set/v3"

type DifferenceOutput[T comparable] struct {
	Added   []T
	Removed []T
}

// before[1,2,3] + [4] -> currentState[1,2,3,4] = Before|currentState [4]
// before[1,2,3] - [3] -> currentState[1,2,4] = currentState|before [3]
func Difference[T comparable](currentState *set.Set[T], NewEntries []T, ToRemoveEntries []T) DifferenceOutput[T] {
	beforeUpdate := currentState.Copy()
	currentState.InsertSlice(NewEntries)
	currentState.RemoveSlice(ToRemoveEntries)
	added := currentState.Difference(beforeUpdate)
	removed := beforeUpdate.Difference(currentState)
	return DifferenceOutput[T]{
		Added:   added.Slice(),
		Removed: removed.Slice(),
	}
}
