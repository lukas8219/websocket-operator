package diff_test

import (
	"encoding/json"
	"log"
	"lukas8219/websocket-operator/internal/diff"
	"os"
	"testing"

	"github.com/hashicorp/go-set/v3"
)

type NodeUpdate struct {
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	New      []string `json:"new"`
	ToDelete []string `json:"toDelete"`
}

func TestDifferenceOuputOnAtomicUpsert(t *testing.T) {
	jsonData, error := os.ReadFile("./expected_state_changes.json")
	if error != nil {
		panic(error)
	}
	var updates []NodeUpdate
	if err := json.Unmarshal(jsonData, &updates); err != nil {
		log.Fatal(err)
	}
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

//
// New Host State Diff added=[10.244.0.146:3000] removed=[10.244.0.142:3000] new="[10.244.0.128 10.244.0.129 10.244.0.130 10.244.0.131 10.244.0.132 10.244.0.133 10.244.0.134 10.244.0.135 10.244.0.136 10.244.0.137 10.244.0.138 10.244.0.139 10.244.0.140 10.244.0.141 10.244.0.142 10.244.0.143 10.244.0.144 10.244.0.145 10.244.0.146 10.244.0.147]" toDelete="[10.244.0.128 10.244.0.129 10.244.0.130 10.244.0.131 10.244.0.132 10.244.0.133 10.244.0.134 10.244.0.135 10.244.0.136 10.244.0.137 10.244.0.138 10.244.0.139 10.244.0.140 10.244.0.141 10.244.0.142 10.244.0.143 10.244.0.144 10.244.0.145 10.244.0.147]
