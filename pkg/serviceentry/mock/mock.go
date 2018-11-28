package mock

import (
	"github.com/tetratelabs/istio-route53/pkg/serviceentry"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Store is a mock for stubbing out serviceentry store
type Store struct {
	Result map[string]*serviceentry.Entry
}

// Classify is not implemented
func (s *Store) Classify(host string) serviceentry.Owner {
	return 0
}

// Ours return s.Result
func (s *Store) Ours() map[string]*serviceentry.Entry {
	return s.Result
}

// Theirs returns s.Result
func (s *Store) Theirs() map[string]*serviceentry.Entry {
	return s.Result
}

// Insert is not implemented
func (s *Store) Insert(cr crd.IstioObject) error {
	return nil
}

// Delete is not implemented
func (s *Store) Delete(cr crd.IstioObject) error {
	return nil
}

// OwnerReference is not implemented
func (s *Store) OwnerReference() v1.OwnerReference {
	return v1.OwnerReference{}
}
