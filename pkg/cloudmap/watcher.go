package cloudmap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/aws/aws-sdk-go/service/servicediscovery/servicediscoveryiface"
	"istio.io/api/networking/v1alpha3"

	"github.com/tetratelabs/istio-cloud-map/pkg/infer"
	"github.com/tetratelabs/istio-cloud-map/pkg/provider"
	"github.com/tetratelabs/log"
)

// consts aren't memory addressable in Go
var serviceFilterNamespaceID = servicediscovery.ServiceFilterNameNamespaceId
var filterConditionEquals = servicediscovery.FilterConditionEq

// Use an empty string as the token for long-lived credentials (token only needed if using STS)
// https://pkg.go.dev/github.com/aws/aws-sdk-go/aws/credentials?tab=doc#NewStaticCredentials
const emptyToken = ""

// NewWatcher returns a Cloud Map watcher
func NewWatcher(store provider.Store, region, id, secret string) (provider.Watcher, error) {
	if len(region) == 0 {
		var ok bool
		if region, ok = os.LookupEnv("AWS_REGION"); !ok {
			return nil, errors.New("AWS region must be specified")
		}
	}

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
		return nil, fmt.Errorf("error setting up AWS session: %w", err)
	}
	return &watcher{cloudmap: servicediscovery.New(session), store: store, interval: time.Second * 5}, nil
}

// watcher polls Cloud Map and caches a list of services and their instances
type watcher struct {
	cloudmap servicediscoveryiface.ServiceDiscoveryAPI
	store    provider.Store
	interval time.Duration
}

var _ provider.Watcher = &watcher{}

func (w *watcher) Store() provider.Store {
	return w.store
}

func (w *watcher) Prefix() string {
	return "cloudmap-"
}

// Run the watcher until the context is cancelled
func (w *watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial sync on startup
	w.refreshStore()
	for {
		select {
		case <-ticker.C:
			w.refreshStore()
		case <-ctx.Done():
			return
		}
	}
}

func (w *watcher) refreshStore() {
	log.Info("Syncing Cloud Map store")
	// TODO: allow users to specify namespaces to watch
	nsResp, err := w.cloudmap.ListNamespaces(&servicediscovery.ListNamespacesInput{})
	if err != nil {
		log.Errorf("error retrieving namespace list from Cloud Map: %v", err)
		return
	}
	// We want to continue to use existing store on error
	tempStore := map[string][]*v1alpha3.WorkloadEntry{}
	for _, ns := range nsResp.Namespaces {
		hosts, err := w.hostsForNamespace(ns)
		if err != nil {
			log.Errorf("unable to refresh Cloud Map cache due to error, using existing cache: %v", err)
			return
		}
		// Hosts are "svcName.nsName" so by definition can't be the same across namespaces or services
		for host, eps := range hosts {
			tempStore[host] = eps
		}
	}
	log.Info("Cloud Map store sync successful")
	w.store.Set(tempStore)
}

func (w *watcher) hostsForNamespace(ns *servicediscovery.NamespaceSummary) (map[string][]*v1alpha3.WorkloadEntry, error) {
	hosts := map[string][]*v1alpha3.WorkloadEntry{}
	svcResp, err := w.cloudmap.ListServices(&servicediscovery.ListServicesInput{
		Filters: []*servicediscovery.ServiceFilter{
			{
				Name:      &serviceFilterNamespaceID,
				Values:    []*string{ns.Id},
				Condition: &filterConditionEquals,
						},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error retrieving service list from Cloud Map for namespace %q: %w", *ns.Name, err)
	}
	for _, svc := range svcResp.Services {
		host := fmt.Sprintf("%v.%v", *svc.Name, *ns.Name)
		eps, err := w.endpointsForService(svc, ns)
		if err != nil {
			return nil, err
		}
		log.Infof("%v Endpoints found for %q", len(eps), host)
		hosts[host] = eps
	}
	return hosts, nil
}

func (w *watcher) endpointsForService(svc *servicediscovery.ServiceSummary, ns *servicediscovery.NamespaceSummary) ([]*v1alpha3.WorkloadEntry, error) {
	// TODO: use health filter?
	instOutput, err := w.cloudmap.DiscoverInstances(&servicediscovery.DiscoverInstancesInput{ServiceName: svc.Name, NamespaceName: ns.Name})
	if err != nil {
		return nil, fmt.Errorf("error retrieving instance list from Cloud Map for %q in %q: %w", *svc.Name, *ns.Name, err)
	}
	// Inject host based instance if there are no instances
	if len(instOutput.Instances) == 0 {
		host := fmt.Sprintf("%v.%v", *svc.Name, *ns.Name)
		instOutput.Instances = []*servicediscovery.HttpInstanceSummary{
			{Attributes: map[string]*string{"AWS_INSTANCE_CNAME": &host}},
		}
	}
	return instancesToEndpoints(instOutput.Instances), nil
}

func instancesToEndpoints(instances []*servicediscovery.HttpInstanceSummary) []*v1alpha3.WorkloadEntry {
	eps := make([]*v1alpha3.WorkloadEntry, 0, len(instances))
	for _, inst := range instances {
		ep := instanceToEndpoint(inst)
		if ep != nil {
			eps = append(eps, ep)
		}
	}
	return eps
}

func instanceToEndpoint(instance *servicediscovery.HttpInstanceSummary) *v1alpha3.WorkloadEntry {
	var address string
	if ip, ok := instance.Attributes["AWS_INSTANCE_IPV4"]; ok {
		address = *ip
	} else if cname, ok := instance.Attributes["AWS_INSTANCE_CNAME"]; ok {
		address = *cname
	}
	if address == "" {
		log.Infof("instance %v of %v.%v is of a type that is not currently supported", *instance.InstanceId, *instance.ServiceName, *instance.NamespaceName)
		return nil
	}
	if port, ok := instance.Attributes["AWS_INSTANCE_PORT"]; ok {
		p, err := strconv.Atoi(*port)
		if err == nil {
			return infer.Endpoint(address, uint32(p))
		}
		log.Errorf("error converting Port string %v to int: %v", *port, err)
	}
	log.Infof("no port found for address %v, assuming http (80) and https (443)", address)
	return &v1alpha3.WorkloadEntry{Address: address, Ports: map[string]uint32{"http": 80, "https": 443}}
}
