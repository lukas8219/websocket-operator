package consistent_hashing

type ConsistentHashing[T any] interface {
	Lookup([]byte) (T, error)
	Transaction(Add, Remove []T)
}
