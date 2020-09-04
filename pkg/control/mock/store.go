package mock

import "istio.io/api/networking/v1alpha3"

// Store is a mock store
type Store struct {
	Result map[string][]*v1alpha3.ServiceEntry_Endpoint
}

// Hosts return s.Result
func (s *Store) Hosts() map[string][]*v1alpha3.ServiceEntry_Endpoint {
	return s.Result
}

func (s *Store) Set(map[string][]*v1alpha3.ServiceEntry_Endpoint) {
	return
}
