package rendezvous

import (
	"math"
	"sync"

	"github.com/buraksezer/consistent"
	"github.com/zeebo/xxh3"
)

var FiftyThreeOnes = uint64(0xFFFFFFFFFFFFFFFF >> (64 - 53))
var FiftyThreeZeros = float64(1 << 53)

var ErrInsufficientMemberCount = consistent.ErrInsufficientMemberCount

type Hasher consistent.Hasher

type DefaultHasher struct {
}

func (h *DefaultHasher) Sum64(b []byte) uint64 {
	return xxh3.Hash(b)
}

type WeightedMember struct {
	member string
	weight float64
}

// Config represents a structure to control the rendezvous package.
type Config struct {
	Hasher Hasher
}

// Rendezvous holds the information about the members of the consistent hash circle.
type Rendezvous struct {
	mu sync.RWMutex

	config  Config
	hasher  Hasher
	members map[string]*WeightedMember
	ring    map[uint64]*WeightedMember
}

// New creates and returns a new Rendezvous object
func New(members []WeightedMember, config Config) *Rendezvous {
	r := &Rendezvous{
		config:  config,
		members: make(map[string]*WeightedMember),
		ring:    make(map[uint64]*WeightedMember),
	}

	if config.Hasher == nil {
		// Use the Default Hasher
		r.hasher = &DefaultHasher{}
	} else {
		r.hasher = config.Hasher
	}
	for _, member := range members {
		r.add(member)
	}
	return r
}

func NewDefault() *Rendezvous {
	return New([]WeightedMember{}, Config{})
}

// IntToFloat is a golang port of the python implementation mentioned here
// https://en.wikipedia.org/wiki/Rendezvous_hashing#Weighted_rendezvous_hash
func IntToFloat(value uint64) (float_value float64) {
	return float64((value & FiftyThreeOnes)) / FiftyThreeZeros
}

func (r *Rendezvous) ComputeWeightedScore(m WeightedMember, key []byte) (score float64) {
	hash := r.hasher.Sum64(append([]byte(m.member), key...))
	score = 1.0 / math.Log(IntToFloat(hash))
	return m.weight * score
}

func (r *Rendezvous) LocateKey(key []byte) (member WeightedMember) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lowest_score := 1.0
	for _, _member := range r.members {
		score := r.ComputeWeightedScore(*_member, key)
		if score < lowest_score {
			lowest_score = score
			member = *_member
		}
	}
	return member
}

func (r *Rendezvous) Lookup(node string) string {
	foundNode := r.LocateKey([]byte(node))
	return foundNode.member
}

type byScore []struct {
	string
	float64
}

func (scores byScore) Len() int {
	return len(scores)
}

func (scores byScore) Swap(i, j int) {
	scores[i], scores[j] = scores[j], scores[i]
}

func (scores byScore) Less(i, j int) bool {
	return scores[i].float64 < scores[j].float64
}

func (r *Rendezvous) add(member WeightedMember) {
	r.members[member.member] = &member
}

func (r *Rendezvous) AddMember(member WeightedMember) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members[member.member]; ok {
		// We already have this member. Quit immediately.
		return
	}
	r.add(member)
}

func (r *Rendezvous) Add(node string) {
	r.AddMember(WeightedMember{
		member: node,
		weight: 1.0,
	})
}

func (r *Rendezvous) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.members[name]; !ok {
		// There is no member with that name. Quit immediately.
		return
	}

	delete(r.members, name)
}

// GetMembers returns a thread-safe copy of members.
func (r *Rendezvous) GetNodes() (members []WeightedMember) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a thread-safe copy of member list.
	members = make([]WeightedMember, 0, len(r.members))
	for _, member := range r.members {
		members = append(members, *member)
	}
	return
}
