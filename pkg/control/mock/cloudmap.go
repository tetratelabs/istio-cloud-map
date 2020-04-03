package mock

import "istio.io/api/networking/v1alpha3"

// CMStore is a mock Cloud Map store
type CMStore struct {
	Result map[string][]*v1alpha3.ServiceEntry_Endpoint
}

// Hosts return s.Result
func (s *CMStore) Hosts() map[string][]*v1alpha3.ServiceEntry_Endpoint {
	return s.Result
}
