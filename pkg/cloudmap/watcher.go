package cloudmap

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/pkg/errors"
	"github.com/tetratelabs/istio-cloud-map/pkg/infer"

	"istio.io/api/networking/v1alpha3"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/aws/aws-sdk-go/service/servicediscovery/servicediscoveryiface"
)

// consts aren't memory addressable in Go
var serviceFilterNamespaceID = servicediscovery.ServiceFilterNameNamespaceId
var filterConditionEquals = servicediscovery.FilterConditionEq

// Use an empty string as the token for long-lived credentials (token only needed if using STS)
// https://pkg.go.dev/github.com/aws/aws-sdk-go/aws/credentials?tab=doc#NewStaticCredentials
const emptyToken = ""

// NewWatcher returns a Cloud Map watcher
func NewWatcher(store Store, region, id, secret string) (*Watcher, error) {
	var creds *credentials.Credentials
	if len(id) == 0 || len(secret) == 0 {
		creds = credentials.NewEnvCredentials()
	} else {
		creds = credentials.NewStaticCredentials(id, secret, emptyToken)
	}

	session, err := session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(region),
	})
	if err != nil {
		return nil, errors.Wrap(err, "error setting up AWS session")
	}
	return &Watcher{cloudmap: servicediscovery.New(session), store: store, interval: time.Second * 5}, nil
}

// Watcher polls Cloud Map and caches a list of services and their instances
type Watcher struct {
	cloudmap servicediscoveryiface.ServiceDiscoveryAPI
	store    Store
	interval time.Duration
}

// Run the watcher until the context is cancelled
func (w *Watcher) Run(ctx context.Context) {
	tick := time.NewTicker(w.interval).C
	// Initial sync on startup
	w.refreshStore()
	for {
		select {
		case <-tick:
			w.refreshStore()
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) refreshStore() {
	log.Print("Syncing Cloud Map store")
	// TODO: allow users to specify namespaces to watch
	nsResp, err := w.cloudmap.ListNamespaces(&servicediscovery.ListNamespacesInput{})
	if err != nil {
		log.Printf("error retrieving namespace list from Cloud Map: %v", err)
		return
	}
	// We want to continue to use existing store on error
	tempStore := map[string][]*v1alpha3.ServiceEntry_Endpoint{}
	for _, ns := range nsResp.Namespaces {
		hosts, err := w.hostsForNamespace(ns)
		if err != nil {
			log.Printf("unable to refresh Cloud Map cache due to error, using existing cache: %v", err)
			return
		}
		// Hosts are "svcName.nsName" so by definition can't be the same across namespaces or services
		for host, eps := range hosts {
			tempStore[host] = eps
		}
	}
	log.Print("Cloud Map store sync successful")
	w.store.(*store).set(tempStore)
}

func (w *Watcher) hostsForNamespace(ns *servicediscovery.NamespaceSummary) (map[string][]*v1alpha3.ServiceEntry_Endpoint, error) {
	hosts := map[string][]*v1alpha3.ServiceEntry_Endpoint{}
	svcResp, err := w.cloudmap.ListServices(&servicediscovery.ListServicesInput{
		Filters: []*servicediscovery.ServiceFilter{
			&servicediscovery.ServiceFilter{
				Name:      &serviceFilterNamespaceID,
				Values:    []*string{ns.Id},
				Condition: &filterConditionEquals,
			},
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving service list from Cloud Map for namespace %q", *ns.Name)
	}
	for _, svc := range svcResp.Services {
		host := fmt.Sprintf("%v.%v", *svc.Name, *ns.Name)
		eps, err := w.endpointsForService(svc, ns)
		if err != nil {
			return nil, err
		}
		log.Printf("%v Endpoints found for %q", len(eps), host)
		hosts[host] = eps
	}
	return hosts, nil
}

func (w *Watcher) endpointsForService(svc *servicediscovery.ServiceSummary, ns *servicediscovery.NamespaceSummary) ([]*v1alpha3.ServiceEntry_Endpoint, error) {
	// TODO: use health filter?
	instOutput, err := w.cloudmap.DiscoverInstances(&servicediscovery.DiscoverInstancesInput{ServiceName: svc.Name, NamespaceName: ns.Name})
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving instance list from Cloud Map for %q in %q", *svc.Name, *ns.Name)
	}
	// Inject host based instance if there are no instances
	if len(instOutput.Instances) == 0 {
		host := fmt.Sprintf("%v.%v", *svc.Name, *ns.Name)
		instOutput.Instances = []*servicediscovery.HttpInstanceSummary{
			&servicediscovery.HttpInstanceSummary{Attributes: map[string]*string{"AWS_INSTANCE_CNAME": &host}},
		}
	}
	return instancesToEndpoints(instOutput.Instances), nil
}

func instancesToEndpoints(instances []*servicediscovery.HttpInstanceSummary) []*v1alpha3.ServiceEntry_Endpoint {
	eps := []*v1alpha3.ServiceEntry_Endpoint{}
	for _, inst := range instances {
		ep := instanceToEndpoint(inst)
		if ep != nil {
			eps = append(eps, ep)
		}
	}
	return eps
}

func instanceToEndpoint(instance *servicediscovery.HttpInstanceSummary) *v1alpha3.ServiceEntry_Endpoint {
	var address string
	if ip, ok := instance.Attributes["AWS_INSTANCE_IPV4"]; ok {
		address = *ip
	} else if cname, ok := instance.Attributes["AWS_INSTANCE_CNAME"]; ok {
		address = *cname
	}
	if address == "" {
		log.Printf("instance %v of %v.%v is of a type that is not currently supported", *instance.InstanceId, *instance.ServiceName, *instance.NamespaceName)
		return nil
	}
	if port, ok := instance.Attributes["AWS_INSTANCE_PORT"]; ok {
		p, err := strconv.Atoi(*port)
		if err == nil {
			return infer.Endpoint(address, uint32(p))
		}
		log.Printf("error converting Port string %v to int: %v", *port, err)
	}
	log.Printf("no port found for address %v, assuming http (80) and https (443)", address)
	return &v1alpha3.ServiceEntry_Endpoint{Address: address, Ports: map[string]uint32{"http": 80, "https": 443}}
}
