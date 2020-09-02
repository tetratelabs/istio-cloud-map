package cloudmap

import (
	"context"
	"log"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"istio.io/api/networking/v1alpha3"

	"github.com/tetratelabs/istio-cloud-map/pkg/infer"
)

const defaultTickInterval = time.Second * 5

// TODO: extract Watcher, Store to pkg/provider interface

type Watcher struct {
	client   *api.Client
	store    *Store
	interval time.Duration
}

func NewWatcher(store *Store, client *api.Client) *Watcher {
	// TODO: make interval configurable
	return &Watcher{client: client, store: store, interval: defaultTickInterval}
}

// Run the watcher until the context is cancelled
func (w *Watcher) Run(ctx context.Context) {
	tick := time.NewTicker(w.interval).C

	w.refreshStore() // init

	// TODO: cache checks
	for {
		select {
		case <-tick:
			w.refreshStore()
		case <-ctx.Done():
			return
		}
	}
}

// fetch services and endpoints from consul catalog and sync them with Store
func (w *Watcher) refreshStore() {
	names, err := w.listServices()
	if err != nil {
		log.Printf("error listing services from Consul: %v", err)
		return
	}

	css, err := w.describeServices(names)
	if err != nil {
		log.Printf("error describing service catalog from Consul:%w ", err)
		return
	}

	data := make(map[string][]*v1alpha3.ServiceEntry_Endpoint, len(css))
	for name, cs := range css {
		eps := make([]*v1alpha3.ServiceEntry_Endpoint, 0, len(cs))
		for _, c := range cs {
			if ep := catalogServiceToEndpoints(c); ep != nil {
				eps = append(eps, ep)
			}
		}
		if len(eps) > 0 {
			data[name] = eps
		}
	}
	w.store.set(data)
}

// listServices lists services
func (w *Watcher) listServices() (map[string][]string, error) {
	data, _, err := w.client.Catalog().Services(nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list services")
	}
	return data, nil
}

// describeServices gets catalog services for given service names
func (w *Watcher) describeServices(names map[string][]string) (map[string][]*api.CatalogService, error) {
	ss := make(map[string][]*api.CatalogService, len(names))
	for name := range names { // ignore tags in value
		svcs, _, err := w.client.Catalog().Service(name, "", nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to describe svc: %s", name)
		}
		ss[name] = svcs
	}
	return ss, nil
}

// catalogServiceToEndpoints converts catalog service to service entry endpoint
func catalogServiceToEndpoints(c *api.CatalogService) *v1alpha3.ServiceEntry_Endpoint {
	address := c.Address
	if address == "" {
		log.Printf("instance %s of %s.%v is of a type that is not currently supported",
			c.ServiceID, c.ServiceName, c.Namespace)
		return nil
	}

	port := c.ServicePort
	if port > 0 { // port is optional and defaults to zero
		return infer.Endpoint(address, uint32(port))
	}

	log.Printf("no port found for address %v, assuming http (80) and https (443)", address)
	return &v1alpha3.ServiceEntry_Endpoint{Address: address, Ports: map[string]uint32{"http": 80, "https": 443}}
}
