package cloudmap

import (
	"sync"

	"istio.io/api/networking/v1alpha3"
)

type (
	Store struct {
		m     *sync.RWMutex
		hosts map[string][]*v1alpha3.ServiceEntry_Endpoint // maps host->Endpoints
	}
)

// NewStore returns a store for Cloud Map data which implements control.Store
func NewStore() *Store {
	return &Store{
		hosts: make(map[string][]*v1alpha3.ServiceEntry_Endpoint),
		m:     &sync.RWMutex{},
	}
}

func (s *Store) Hosts() map[string][]*v1alpha3.ServiceEntry_Endpoint {
	s.m.RLock()
	defer s.m.RUnlock()
	return copyMap(s.hosts)
}

func (s *Store) set(hosts map[string][]*v1alpha3.ServiceEntry_Endpoint) {
	s.m.Lock()
	defer s.m.Unlock()
	s.hosts = copyMap(hosts)
}

func copyMap(m map[string][]*v1alpha3.ServiceEntry_Endpoint) map[string][]*v1alpha3.ServiceEntry_Endpoint {
	out := make(map[string][]*v1alpha3.ServiceEntry_Endpoint, len(m))
	for k, v := range m {
		eps := make([]*v1alpha3.ServiceEntry_Endpoint, len(v))
		copy(eps, v)
		out[k] = eps
	}
	return out
}
