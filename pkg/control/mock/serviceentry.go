package mock

import (
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tetratelabs/istio-cloud-map/pkg/serviceentry"
)

// SEStore is a mock for stubbing out serviceentry store
type SEStore struct {
	Result map[string]*v1alpha3.ServiceEntry
}

// Classify is not implemented
func (s *SEStore) Classify(host string) serviceentry.Owner {
	return 0
}

// Ours return s.Result
func (s *SEStore) Ours() map[string]*v1alpha3.ServiceEntry {
	return s.Result
}

// Theirs returns s.Result
func (s *SEStore) Theirs() map[string]*v1alpha3.ServiceEntry {
	return s.Result
}

// Insert is not implemented
func (s *SEStore) Insert(se *v1alpha3.ServiceEntry) error {
	return nil
}

func (s *SEStore) Update(_, _ *v1alpha3.ServiceEntry) error {
	return nil
}

// Delete is not implemented
func (s *SEStore) Delete(se *v1alpha3.ServiceEntry) error {
	return nil
}

// OwnerReference is not implemented
func (s *SEStore) OwnerReference() v1.OwnerReference {
	return v1.OwnerReference{}
}
