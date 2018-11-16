package route53

import (
	"sync"
)

type (
	// Store describes a set of endpoint objects from Cloud Map stored by the hostnames that own them.
	Store interface {
		// Hosts are all hosts Cloud Map has told us about
		Hosts() map[string][]endpoint

		// Set updates the cache to reflect the new state
		// Private to ensure this store is read-only outside of this package
		set(hosts map[string][]endpoint)
	}

	store struct {
		m     *sync.RWMutex
		hosts map[string][]endpoint // maps host->endpoints
	}
)

// New returns a new store which stores Cloud Map data
func New() Store {
	return &store{
		hosts: make(map[string][]endpoint),
		m:     &sync.RWMutex{},
	}
}

func (s *store) Hosts() map[string][]endpoint {
	s.m.RLock()
	defer s.m.RUnlock()
	return copyMap(s.hosts)
}

func (s *store) set(hosts map[string][]endpoint) {
	s.m.Lock()
	defer s.m.Unlock()
	s.hosts = copyMap(hosts)
}

func copyMap(m map[string][]endpoint) map[string][]endpoint {
	out := make(map[string][]endpoint, len(m))
	for k, v := range m {
		eps := make([]endpoint, 0) // len 0 forces new slice creation when appending
		eps = append(eps, v...)
		out[k] = eps
	}
	return out
}
