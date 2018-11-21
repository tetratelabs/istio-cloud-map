package route53

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/pkg/errors"
	"github.com/tetratelabs/istio-route53/pkg/infer"
	"istio.io/api/networking/v1alpha3"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/aws/aws-sdk-go/service/servicediscovery/servicediscoveryiface"
)

// consts aren't memory addressable in Go
var serviceFilterNamespaceID = servicediscovery.ServiceFilterNameNamespaceId
var filterConditionEquals = servicediscovery.FilterConditionEq

// NewWatcher returns a Route53 watcher
func NewWatcher(store Store) (*Watcher, error) {
	session, err := session.NewSession(&aws.Config{
		// TODO: env vars aren't a secure way to pass secrets
		Credentials: credentials.NewEnvCredentials(),

		// TODO: don't hardcode region
		Region: aws.String("us-west-2"),
	})
	if err != nil {
		return nil, errors.Wrap(err, "error setting up AWS session")

	}

	r53 := servicediscovery.New(session)
	cloudmap := servicediscovery.New(session)
	cloudmap.Endpoint = "https://data-servicediscovery.us-west-2.amazonaws.com"

	return &Watcher{r53: r53, cloudmap: cloudmap, store: store, interval: time.Second * 5}, nil
}

// Watcher polls Route53 and caches a list of services and their instances
type Watcher struct {
	r53      servicediscoveryiface.ServiceDiscoveryAPI
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
	log.Print("Syncing Route53 store")
	// TODO: allow users to specify namespaces to watch
	nsResp, err := w.r53.ListNamespaces(&servicediscovery.ListNamespacesInput{})
	if err != nil {
		log.Printf("error retrieving namespace list from Route53: %v", err)
		return
	}
	// We want to continue to use existing store on error
	tempStore := map[string][]*v1alpha3.ServiceEntry_Endpoint{}
	for _, ns := range nsResp.Namespaces {
		hosts, err := w.hostsForNamespace(ns)
		if err != nil {
			log.Printf("unable to refresh route 53 cache due to error, using existing cache: %v", err)
			return
		}
		// Hosts are "svcName.nsName" so by definition can't be the same across namespaces or services
		for host, eps := range hosts {
			tempStore[host] = eps
		}
	}
	log.Print("Route53 store sync successful")
	w.store.(*store).set(tempStore)
}

func (w *Watcher) hostsForNamespace(ns *servicediscovery.NamespaceSummary) (map[string][]*v1alpha3.ServiceEntry_Endpoint, error) {
	hosts := map[string][]*v1alpha3.ServiceEntry_Endpoint{}
	svcResp, err := w.r53.ListServices(&servicediscovery.ListServicesInput{
		Filters: []*servicediscovery.ServiceFilter{
			&servicediscovery.ServiceFilter{
				Name:      &serviceFilterNamespaceID,
				Values:    []*string{ns.Id},
				Condition: &filterConditionEquals,
			},
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving service list from Route53 for namespace %q", *ns.Name)
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
		return nil, errors.Wrapf(err, "error retrieving instance list from Route53 for %q in %q", *svc.Name, *ns.Name)
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
