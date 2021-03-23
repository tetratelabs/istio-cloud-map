package provider

import (
	"sync"

	"istio.io/api/networking/v1alpha3"
)

type (
	// Store describes a set of Istio endpoint objects from Cloud Map/Consul stored by the hostnames that own them.
	// It is asynchronously accessed by a provider and the synchronizer
	Store interface {
		// Hosts are all hosts Cloud Map/Consul has told us about
		Hosts() map[string][]*v1alpha3.WorkloadEntry
		Set(hosts map[string][]*v1alpha3.WorkloadEntry)
	}

	store struct {
		m     *sync.RWMutex
		hosts map[string][]*v1alpha3.WorkloadEntry // maps host->Endpoints
	}
)

// NewStore returns a store
func NewStore() Store {
	return &store{
		hosts: make(map[string][]*v1alpha3.WorkloadEntry),
		m:     &sync.RWMutex{},
	}
}

func (s *store) Hosts() map[string][]*v1alpha3.WorkloadEntry {
	s.m.RLock()
	defer s.m.RUnlock()
	return copyMap(s.hosts)
}

func (s *store) Set(hosts map[string][]*v1alpha3.WorkloadEntry) {
	s.m.Lock()
	defer s.m.Unlock()
	s.hosts = copyMap(hosts)
}

func copyMap(m map[string][]*v1alpha3.WorkloadEntry) map[string][]*v1alpha3.WorkloadEntry {
	out := make(map[string][]*v1alpha3.WorkloadEntry, len(m))
	for k, v := range m {
		eps := make([]*v1alpha3.WorkloadEntry, len(v))
		copy(eps, v)
		out[k] = eps
	}
	return out
}
