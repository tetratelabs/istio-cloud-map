// Copyright 2018 Tetrate Labs
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package serviceentry

import (
	"fmt"
	"reflect"
	"sync"

	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"istio.io/istio/pilot/pkg/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

type (
	// Owner describes which system owns the resource in question
	Owner int

	// Store describes a set of ServiceEntry objects stored by the hostnames they claim.
	Store interface {
		// Based on the data in the store, classify the host as belonging to us, them, or no one.
		Classify(host string) Owner

		// Ours are ServiceEntries managed by us
		Ours() map[string]*Entry

		// Theirs are ServiceEntries managed by any other system
		Theirs() map[string]*Entry

		// Insert adds a ServiceEntry to the store (detecting who it belongs to)
		Insert(cr crd.IstioObject) error

		// Delete removes a ServiceEntry from the store
		Delete(cr crd.IstioObject) error
	}

	Entry struct {
		meta v1.ObjectMeta
		spec *v1alpha3.ServiceEntry
	}

	store struct {
		ref          v1.OwnerReference
		m            sync.RWMutex      // guards both maps
		ours, theirs map[string]*Entry // maps host->Entry; a single Entry can be referenced by many hosts
	}
)

const (
	// Us means we own the resource
	Us Owner = iota
	// Them means they own the resource
	Them Owner = iota
	// None means no one owns the resource
	None Owner = iota
)

// New returns a new store which manages resources marked by the provided ID
func New(id string) Store {
	return &store{
		ref:    ownerRef(id),
		ours:   make(map[string]*Entry),
		theirs: make(map[string]*Entry),
	}
}

// Classify the host as belonging to us, them, or no one
func (s *store) Classify(host string) Owner {
	s.m.RLock()
	defer s.m.RUnlock()

	if _, found := s.ours[host]; found {
		return Us
	}
	if _, found := s.theirs[host]; found {
		return Them
	}
	return None
}

func (s *store) Ours() map[string]*Entry {
	s.m.RLock()
	defer s.m.RUnlock()
	return copyMap(s.ours)
}

func (s *store) Theirs() map[string]*Entry {
	s.m.RLock()
	defer s.m.RUnlock()
	return copyMap(s.theirs)
}

func (s *store) Insert(cr crd.IstioObject) error {
	cfg, err := crd.ConvertObject(model.ServiceEntry, cr, "")
	if err != nil {
		return fmt.Errorf("failed to convert IstioObject to ServiceEntry")
	}

	owner := owner(s.ref, cr.GetObjectMeta().OwnerReferences)
	entry := &Entry{
		meta: cr.GetObjectMeta(),
		spec: cfg.Spec.(*v1alpha3.ServiceEntry),
	}
	// as a single update, we insert all hosts owned by the ServiceEntry
	s.m.Lock()
	switch owner {
	case None, Us:
		for _, host := range entry.spec.Hosts {
			s.ours[host] = entry
		}
	case Them:
		for _, host := range entry.spec.Hosts {
			s.theirs[host] = entry
		}
	}
	s.m.Unlock()
	return nil
}

func (s *store) Delete(cr crd.IstioObject) error {
	cfg, err := crd.ConvertObject(model.ServiceEntry, cr, "")
	if err != nil {
		return fmt.Errorf("failed to convert IstioObject to ServiceEntry")
	}
	owner := owner(s.ref, cr.GetObjectMeta().OwnerReferences)
	// as a single update, we delete all hosts owned by the ServiceEntry
	s.m.Lock()
	switch owner {
	case Us:
		for _, host := range cfg.Spec.(*v1alpha3.ServiceEntry).Hosts {
			delete(s.ours, host)
		}
	case Them:
		for _, host := range cfg.Spec.(*v1alpha3.ServiceEntry).Hosts {
			delete(s.theirs, host)
		}
	case None:
		// for those with no owner, make sure we remove from both maps
		for _, host := range cfg.Spec.(*v1alpha3.ServiceEntry).Hosts {
			delete(s.ours, host)
		}
		for _, host := range cfg.Spec.(*v1alpha3.ServiceEntry).Hosts {
			delete(s.theirs, host)
		}
	}
	s.m.Unlock()
	return nil
}

func owner(self v1.OwnerReference, refs []v1.OwnerReference) Owner {
	if len(refs) == 0 {
		return None
	}
	for _, ref := range refs {
		if reflect.DeepEqual(ref, self) {
			return Us
		}
	}
	// there's some owner reference but it wasn't ours
	return Them
}

func copyMap(m map[string]*Entry) map[string]*Entry {
	out := make(map[string]*Entry, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func ownerRef(id string) v1.OwnerReference {
	t := true
	return v1.OwnerReference{
		APIVersion: "route53.istio.io",
		Kind:       "ServiceController",
		Name:       id,
		Controller: &t,
	}
}

// LoggingStore wraps another Store and logs its operations using the log function.
type LoggingStore struct {
	log func(fmt string, args ...interface{})
	s   Store
}

// Wraps the underlying Store and logs operations perfomed on it using the supplied functions.
// log.Printf satisfies the signature for log.
func NewLoggingStore(s Store, log func(fmt string, args ...interface{})) Store {
	return LoggingStore{log, s}
}

// Based on the data in the store, classify the host as belonging to us, them, or no one.
func (l LoggingStore) Classify(host string) Owner {
	o := l.s.Classify(host)
	l.log("classified %q as %d", host, o)
	return o
}

// Ours are ServiceEntries managed by us
func (l LoggingStore) Ours() map[string]*Entry {
	ours := l.s.Ours()
	l.log("returned ours map: %v", ours)
	return ours
}

// Theirs are ServiceEntries managed by any other system
func (l LoggingStore) Theirs() map[string]*Entry {
	theirs := l.s.Theirs()
	l.log("returned ours map: %v", theirs)
	return theirs
}

// Insert adds a ServiceEntry to the store (detecting who it belongs to)
func (l LoggingStore) Insert(cr crd.IstioObject) error {
	err := l.s.Insert(cr)
	l.log("inserted %v with result %v", cr, err)
	return err
}

// Delete removes a ServiceEntry from the store
func (l LoggingStore) Delete(cr crd.IstioObject) error {
	err := l.s.Delete(cr)
	l.log("deleted %v with result %v", cr, err)
	return err
}
