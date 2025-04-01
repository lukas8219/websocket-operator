// Straight copy from https://github.com/dgryski/go-rendezvous
// I'll need to tweak it in many places
package rendezvous

type Rendezvous struct {
	nodes map[string]int
	nstr  []string
	nhash []uint64
	hash  Hasher
}

type Hasher func(s string) uint64

func New(nodes []string, hash Hasher) *Rendezvous {
	r := &Rendezvous{
		nodes: make(map[string]int, len(nodes)),
		nstr:  make([]string, len(nodes)),
		nhash: make([]uint64, len(nodes)),
		hash:  hash,
	}

	for i, n := range nodes {
		r.nodes[n] = i
		r.nstr[i] = n
		r.nhash[i] = hash(n)
	}

	return r
}

func (r *Rendezvous) Lookup(k string) string {
	// short-circuit if we're empty
	if len(r.nodes) == 0 {
		return ""
	}

	khash := r.hash(k)

	var midx int
	var mhash = xorshiftMult64(khash ^ r.nhash[0])

	for i, nhash := range r.nhash[1:] {
		if h := xorshiftMult64(khash ^ nhash); h > mhash {
			midx = i + 1
			mhash = h
		}
	}

	return r.nstr[midx]
}

func (r *Rendezvous) Add(node string) {
	r.nodes[node] = len(r.nstr)
	r.nstr = append(r.nstr, node)
	r.nhash = append(r.nhash, r.hash(node))
}

func (r *Rendezvous) HasNode(node string) bool {
	return r.nodes[node] != 0
}

func (r *Rendezvous) GetNodes() []string {
	return r.nstr
}

func (r *Rendezvous) Remove(node string) {
	if len(r.nodes) == 0 {
		return
	}
	// find index of node to remove
	nidx := r.nodes[node]
	last := len(r.nstr) - 1

	// if not removing last node, swap last into nidx
	if nidx != last {
		r.nstr[nidx] = r.nstr[last]
		r.nhash[nidx] = r.nhash[last]
		moved := r.nstr[nidx]
		r.nodes[moved] = nidx
	}

	// truncate slices
	r.nstr = r.nstr[:last]
	r.nhash = r.nhash[:last]

	// delete from map
	delete(r.nodes, node)
}

func xorshiftMult64(x uint64) uint64 {
	x ^= x >> 12 // a
	x ^= x << 25 // b
	x ^= x >> 27 // c
	return x * 2685821657736338717
}
