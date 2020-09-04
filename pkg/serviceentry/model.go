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
	"reflect"
	"sync"

	"github.com/golang/protobuf/proto"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tetratelabs/log"
)

type (
	// Owner describes which system owns the resource in question
	Owner int

	// Store describes a set of ServiceEntry objects stored by the hostnames they claim.
	Store interface {
		// Based on the data in the store, classify the host as belonging to us, them, or no one.
		Classify(host string) Owner

		// Ours are ServiceEntries managed by us
		Ours() map[string]*v1alpha3.ServiceEntry

		// Theirs are ServiceEntries managed by any other system
		Theirs() map[string]*v1alpha3.ServiceEntry

		// Insert adds a ServiceEntry to the store (detecting who it belongs to)
		Insert(se *v1alpha3.ServiceEntry) error

		// Update updates a ServiceEntry's claimed hosts in the store
		Update(old, newse *v1alpha3.ServiceEntry) error

		// Delete removes a ServiceEntry from the store
		Delete(se *v1alpha3.ServiceEntry) error

		// OwnerReference is used to label new entries as owned by this store
		OwnerReference() v1.OwnerReference
	}

	store struct {
		ref          v1.OwnerReference
		m            sync.RWMutex                      // guards both maps
		ours, theirs map[string]*v1alpha3.ServiceEntry // maps host->Entry; a single Entry can be referenced by many hosts
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
func New(ownerRef v1.OwnerReference) Store {
	return &store{
		ref:    ownerRef,
		ours:   make(map[string]*v1alpha3.ServiceEntry),
		theirs: make(map[string]*v1alpha3.ServiceEntry),
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

func (s *store) Ours() map[string]*v1alpha3.ServiceEntry {
	s.m.RLock()
	defer s.m.RUnlock()
	return copyMap(s.ours)
}

func (s *store) OwnerReference() v1.OwnerReference {
	return s.ref
}

func (s *store) Theirs() map[string]*v1alpha3.ServiceEntry {
	s.m.RLock()
	defer s.m.RUnlock()
	return copyMap(s.theirs)
}

func (s *store) Insert(se *v1alpha3.ServiceEntry) error {
	owner := owner(s.ref, se.GetOwnerReferences())
	// as a single update, we insert all hosts owned by the ServiceEntry
	s.m.Lock()
	s.add(owner, se)
	s.m.Unlock()
	return nil
}

func (s *store) Update(old, se *v1alpha3.ServiceEntry) error {
	if proto.Equal(&old.Spec, &se.Spec) {
		log.Infof("skipping update, no change")
		return nil
	}

	oldOwner := owner(s.ref, old.GetOwnerReferences())
	owner := owner(s.ref, se.GetOwnerReferences())

	s.m.Lock()
	s.delete(oldOwner, old)
	s.add(owner, se)
	s.m.Unlock()
	return nil
}

func (s *store) Delete(se *v1alpha3.ServiceEntry) error {
	owner := owner(s.ref, se.GetObjectMeta().GetOwnerReferences())
	// as a single update, we delete all hosts owned by the ServiceEntry
	s.m.Lock()
	s.delete(owner, se)
	s.m.Unlock()
	return nil
}

func (s *store) add(owner Owner, se *v1alpha3.ServiceEntry) {
	switch owner {
	case None, Us:
		for _, host := range se.Spec.Hosts {
			s.ours[host] = se
		}
	case Them:
		for _, host := range se.Spec.Hosts {
			s.theirs[host] = se
		}
	}
}

func (s *store) delete(owner Owner, se *v1alpha3.ServiceEntry) {
	switch owner {
	case Us:
		for _, host := range se.Spec.Hosts {
			delete(s.ours, host)
		}
	case Them:
		for _, host := range se.Spec.Hosts {
			delete(s.theirs, host)
		}
	case None:
		// for those with no owner, make sure we remove from both maps
		for _, host := range se.Spec.Hosts {
			delete(s.ours, host)
		}
		for _, host := range se.Spec.Hosts {
			delete(s.theirs, host)
		}
	}
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

func copyMap(m map[string]*v1alpha3.ServiceEntry) map[string]*v1alpha3.ServiceEntry {
	out := make(map[string]*v1alpha3.ServiceEntry, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
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

func (l LoggingStore) OwnerReference() v1.OwnerReference {
	return l.s.OwnerReference()
}

// Based on the data in the store, classify the host as belonging to us, them, or no one.
func (l LoggingStore) Classify(host string) Owner {
	o := l.s.Classify(host)
	l.log("classified %q as %d", host, o)
	return o
}

// Ours are ServiceEntries managed by us
func (l LoggingStore) Ours() map[string]*v1alpha3.ServiceEntry {
	ours := l.s.Ours()
	l.log("returned ours map: %v", ours)
	return ours
}

// Theirs are ServiceEntries managed by any other system
func (l LoggingStore) Theirs() map[string]*v1alpha3.ServiceEntry {
	theirs := l.s.Theirs()
	l.log("returned ours map: %v", theirs)
	return theirs
}

// Insert adds a ServiceEntry to the store (detecting who it belongs to)
func (l LoggingStore) Insert(se *v1alpha3.ServiceEntry) error {
	err := l.s.Insert(se)
	l.log("inserted %v with result %v", se, err)
	return err
}

func (l LoggingStore) Update(old, se *v1alpha3.ServiceEntry) error {
	err := l.s.Update(old, se)
	l.log("updated %v to %v with result %v", old, se, err)
	return err
}

// Delete removes a ServiceEntry from the store
func (l LoggingStore) Delete(se *v1alpha3.ServiceEntry) error {
	err := l.s.Delete(se)
	l.log("deleted %v with result %v", se, err)
	return err
}
