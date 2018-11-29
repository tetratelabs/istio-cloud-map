package control

import (
	"context"
	"log"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/tetratelabs/istio-cloud-map/pkg/cloudmap"
	"github.com/tetratelabs/istio-cloud-map/pkg/infer"
	"github.com/tetratelabs/istio-cloud-map/pkg/serviceentry"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"istio.io/istio/pilot/pkg/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

type synchronizer struct {
	owner        v1.OwnerReference
	serviceEntry serviceentry.Store
	cloudMap     cloudmap.Store
	istio        model.ConfigStore
	interval     time.Duration
}

func NewSynchronizer(owner v1.OwnerReference, serviceEntry serviceentry.Store, cloudMap cloudmap.Store, kubeConfig string) (*synchronizer, error) {
	istio, err := crd.NewClient(kubeConfig, "", model.IstioConfigTypes, "")
	return &synchronizer{
		owner:        owner,
		serviceEntry: serviceEntry,
		cloudMap:     cloudMap,
		istio:        istio,
		interval:     time.Second * 5,
	}, errors.Wrap(err, "unable to create synchronizer")
}

// Run the synchronizer until the context is cancelled
func (s *synchronizer) Run(ctx context.Context) {
	tick := time.NewTicker(s.interval).C
	for {
		select {
		case <-tick:
			s.sync()
		case <-ctx.Done():
			return
		}
	}
}

func (s *synchronizer) sync() {
	// Entries are generated per host; entirely from information in the slice of endpoints;
	// so we only actually need to compare the current endpoints with the new endpoints.
	for host, endpoints := range s.cloudMap.Hosts() {
		// If a service entry with the same host has been created by someone else, continue.
		if _, ok := s.serviceEntry.Theirs()[host]; ok {
			continue
		}
		s.createOrUpdate(host, endpoints)
	}
	s.garbageCollect()
}

func (s *synchronizer) createOrUpdate(host string, endpoints []*v1alpha3.ServiceEntry_Endpoint) {
	newServiceEntry := infer.ServiceEntry(s.owner, host, endpoints)
	if _, ok := s.serviceEntry.Ours()[host]; ok {
		// If we have already created an identical service entry, return.
		if reflect.DeepEqual(s.serviceEntry.Ours()[host].Endpoints(), endpoints) {
			return
		}
		// Otherwise, endpoints have changed so update existing Service Entry
		oldServiceEntry, found := s.istio.Get(model.ServiceEntry.Type, infer.ServiceEntryName(host), "istio-system")
		if !found {
			return
		}
		newServiceEntry.ResourceVersion = oldServiceEntry.ResourceVersion
		rv, err := s.istio.Update(newServiceEntry)
		if err != nil {
			log.Printf("error updating Service Entry %q: %v", infer.ServiceEntryName(host), err)
			return
		}
		log.Printf("updated Service Entry %q, ResourceVersion is now %q", infer.ServiceEntryName(host), rv)
		return
	}
	// Otherwise, create a new Service Entry
	rv, err := s.istio.Create(newServiceEntry)
	if err != nil {
		log.Printf("error creating Service Entry %q: %v", infer.ServiceEntryName(host), err)
	}
	log.Printf("created Service Entry %q, ResourceVersion is %q", infer.ServiceEntryName(host), rv)
}

func (s *synchronizer) garbageCollect() {
	for host := range s.serviceEntry.Ours() {
		// If host no longer exists, delete service entry
		if _, ok := s.cloudMap.Hosts()[host]; !ok {
			// TODO: Don't attempt to delete no owners
			if err := s.istio.Delete(model.ServiceEntry.Type, infer.ServiceEntryName(host), "istio-system"); err != nil {
				log.Printf("error deleting Service Entry %q: %v", infer.ServiceEntryName(host), err)
			}
			log.Printf("successfully deleted Service Entry %q", infer.ServiceEntryName(host))
		}
	}
}
