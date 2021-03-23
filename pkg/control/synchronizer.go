package control

import (
	"context"
	"reflect"
	"time"

	"istio.io/api/networking/v1alpha3"
	icapi "istio.io/client-go/pkg/clientset/versioned/typed/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tetratelabs/istio-cloud-map/pkg/infer"
	"github.com/tetratelabs/istio-cloud-map/pkg/provider"
	"github.com/tetratelabs/istio-cloud-map/pkg/serviceentry"
	"github.com/tetratelabs/log"
)

type synchronizer struct {
	owner              v1.OwnerReference
	serviceEntry       serviceentry.Store
	store              provider.Store
	serviceEntryPrefix string
	client             icapi.ServiceEntryInterface
	interval           time.Duration
}

func NewSynchronizer(owner v1.OwnerReference,
	serviceEntry serviceentry.Store, store provider.Store, serviceEntryPrefix string, client icapi.ServiceEntryInterface) *synchronizer {
	return &synchronizer{
		owner:              owner,
		serviceEntry:       serviceEntry,
		store:              store,
		serviceEntryPrefix: serviceEntryPrefix,
		client:             client,
		interval:           time.Second * 5,
	}
}

// Run the synchronizer until the context is cancelled
func (s *synchronizer) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.sync(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *synchronizer) sync(ctx context.Context) {
	// Entries are generated per host; entirely from information in the slice of endpoints;
	// so we only actually need to compare the current endpoints with the new endpoints.
	for host, endpoints := range s.store.Hosts() {
		// If a service entry with the same host has been created by someone else, continue.
		if _, ok := s.serviceEntry.Theirs()[host]; ok {
			continue
		}
		s.createOrUpdate(ctx, host, endpoints)
	}
	s.garbageCollect(ctx)
}

func (s *synchronizer) createOrUpdate(ctx context.Context, host string, endpoints []*v1alpha3.WorkloadEntry) {
	newServiceEntry := infer.ServiceEntry(s.owner, s.serviceEntryPrefix, host, endpoints)
	name := infer.ServiceEntryName(s.serviceEntryPrefix, host)
	if _, ok := s.serviceEntry.Ours()[host]; ok {
		// If we have already created an identical service entry, return.
		if reflect.DeepEqual(s.serviceEntry.Ours()[host].Spec.Endpoints, endpoints) {
			return
		}
		// Otherwise, endpoints have changed so update existing Service Entry
		n := infer.ServiceEntryName(s.serviceEntryPrefix, host)
		oldServiceEntry, err := s.client.Get(ctx, n, v1.GetOptions{})
		if err != nil {
			log.Errorf("failed to get existing service entry %q for host %q", n, host)
			return
		}
		newServiceEntry.ResourceVersion = oldServiceEntry.ResourceVersion
		rv, err := s.client.Update(ctx, newServiceEntry, v1.UpdateOptions{})
		if err != nil {
			log.Errorf("error updating Service Entry %q: %v", name, err)
			return
		}
		log.Infof("updated Service Entry %q, ResourceVersion is now %q", name, rv.ResourceVersion)
		return
	}
	// Otherwise, create a new Service Entry
	rv, err := s.client.Create(ctx, newServiceEntry, v1.CreateOptions{})
	if err != nil {
		log.Errorf("error creating Service Entry %q: %v\n%v", name, err, newServiceEntry)
	}
	log.Infof("created Service Entry %q, ResourceVersion is %q", name, rv.ResourceVersion)
}

func (s *synchronizer) garbageCollect(ctx context.Context) {
	for host := range s.serviceEntry.Ours() {
		// If host no longer exists, delete service entry
		if _, ok := s.store.Hosts()[host]; !ok {
			// TODO: namespaces!
			// TODO: Don't attempt to delete no owners
			name := infer.ServiceEntryName(s.serviceEntryPrefix, host)
			if err := s.client.Delete(ctx, name, v1.DeleteOptions{}); err != nil {
				log.Errorf("error deleting Service Entry %q: %v", name, err)
			}
			log.Infof("successfully deleted Service Entry %q", name)
		}
	}
}
