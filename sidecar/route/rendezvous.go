package route

import "github.com/dgryski/go-rendezvous"

type Router struct {
	rendezvous rendezvous.Rendezvous
}

func NewRouter() *Router {
	return &Router{
		rendezvous: *rendezvous.New([]string{}, func(s string) uint64 {
			var sum uint64
			for _, c := range s {
				sum += uint64(c)
			}
			return sum
		}),
	}
}

func (r *Router) Route(recipientId string) string {
	return r.rendezvous.Lookup(recipientId)
}

func (r *Router) Add(host []string) {
	if r.rendezvous.Lookup(host[0]) != "" {
		return
	}
	for _, host := range host {
		r.rendezvous.Add(host)
	}
}
