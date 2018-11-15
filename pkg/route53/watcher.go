package route53

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/aws/aws-sdk-go/service/servicediscovery/servicediscoveryiface"
)

// consts aren't memory addressable in Go
var serviceFilterNamespaceID = servicediscovery.ServiceFilterNameNamespaceId
var filterConditionEquals = servicediscovery.FilterConditionEq

// NewWatcher returns a Route53 watcher
func NewWatcher() (*Watcher, error) {
	session, err := session.NewSession(&aws.Config{
		// TODO: env vars aren't a secure way to pass secrets
		Credentials: credentials.NewEnvCredentials(),
		// TODO: don't hardcode region
		Region: aws.String("us-west-2"),
	})
	if err != nil {
		return nil, errors.Wrap(err, "error setting up AWS session")
	}

	r53Client := servicediscovery.New(session)
	cloudMapClient := servicediscovery.New(session)
	cloudMapClient.Endpoint = "https://data-servicediscovery.us-west-2.amazonaws.com"

	return &Watcher{r53: r53Client, cloudmap: cloudMapClient, interval: time.Second * 5}, nil
}

// Watcher polls Route53 and caches a list of services and their instances
type Watcher struct {
	r53      servicediscoveryiface.ServiceDiscoveryAPI
	cloudmap servicediscoveryiface.ServiceDiscoveryAPI
	store    Store
	interval time.Duration
}

type endpoint struct {
	host string
	port int
}

// Run the watcher until the context is cancelled
func (w *Watcher) Run(ctx context.Context) {
	// TODO: delete when Cloud Map UI is fully working
	w.populateDummyData()

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
	tempStore := map[string][]endpoint{}
	for _, ns := range nsResp.Namespaces {
		hosts, err := w.hostsForNamespace(ns)
		if err != nil {
			log.Printf("unable to refresh route 53 cache due to error, using existing cache: %v", err)
			return
		}
		// hosts are "svcName.nsName" so by definition can't be the same across namespaces or services
		for host, eps := range hosts {
			tempStore[host] = eps
		}
	}
	log.Print("Route53 store sync successful")
	w.store.set(tempStore)
}

func (w *Watcher) hostsForNamespace(ns *servicediscovery.NamespaceSummary) (map[string][]endpoint, error) {
	hosts := map[string][]endpoint{}
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
		log.Printf("%v endpoints found for %q", len(eps), host)
		hosts[host] = eps
	}
	return hosts, nil
}

func (w *Watcher) endpointsForService(svc *servicediscovery.ServiceSummary, ns *servicediscovery.NamespaceSummary) ([]endpoint, error) {
	// TODO: use health filter?
	instOutput, err := w.cloudmap.DiscoverInstances(&servicediscovery.DiscoverInstancesInput{ServiceName: svc.Name, NamespaceName: ns.Name})
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving instance list from Route53 for %q in %q", *svc.Name, *ns.Name)
	}
	return instancesToEndpoints(instOutput.Instances), nil
}

func instancesToEndpoints(instances []*servicediscovery.HttpInstanceSummary) []endpoint {
	eps := []endpoint{}
	for _, inst := range instances {
		eps = append(eps, instanceToEndpoint(inst)...)
	}
	return eps
}

func instanceToEndpoint(instance *servicediscovery.HttpInstanceSummary) []endpoint {
	var host string
	if ip, ok := instance.Attributes["AWS_INSTANCE_IPV4"]; ok {
		host = *ip
	}
	if host == "" {
		log.Printf("instance %v of %v.%v is of a type that is not currently supported", *instance.InstanceId, *instance.ServiceName, *instance.NamespaceName)
		return []endpoint{}
	}
	if port, ok := instance.Attributes["AWS_INSTANCE_PORT"]; ok {
		p, err := strconv.Atoi(*port)
		if err == nil {
			return []endpoint{endpoint{host: host, port: p}}
		}
		log.Printf("error converting port string %v to int: %v", *port, err)
	}
	log.Printf("no port found for host %v, assuming http (80) and https (443)", host)
	return []endpoint{
		endpoint{host: host, port: 80},
		endpoint{host: host, port: 443},
	}
}

// TODO: delete when Cloud Map UI is fully working
func (w *Watcher) populateDummyData() {
	nsName := "demo.tetrate.io"
	svcName := "dev.null"
	for {
		// Namespace creation
		nss, err := w.r53.ListNamespaces(&servicediscovery.ListNamespacesInput{})
		if err != nil {
			log.Printf("error listing namespace %v", err)
		}
		if len(nss.Namespaces) == 0 {
			log.Print("no namespaces found creating one")
			if _, err := w.r53.CreateHttpNamespace(&servicediscovery.CreateHttpNamespaceInput{Name: &nsName}); err != nil {
				log.Printf("error creating http namespace %v", err)
			}
			time.Sleep(time.Second * 10)
			continue
		}
		log.Print(nss)
		// Service creation
		svcs, err := w.r53.ListServices(&servicediscovery.ListServicesInput{})
		if err != nil {
			log.Printf("error listing service %v", err)
		}
		if len(svcs.Services) == 0 {
			log.Print("no services found creating one")
			if _, err := w.r53.CreateService(&servicediscovery.CreateServiceInput{NamespaceId: nss.Namespaces[0].Id, Name: &svcName}); err != nil {
				log.Printf("error creating service %v", err)
			}
			time.Sleep(time.Second * 10)
			continue
		}
		log.Print(svcs)
		// Instance registration
		insts, err := w.cloudmap.DiscoverInstances(&servicediscovery.DiscoverInstancesInput{NamespaceName: nss.Namespaces[0].Name, ServiceName: svcs.Services[0].Name})
		if err != nil {
			log.Printf("error discovering instances %v", err)
		}
		if len(insts.Instances) > 0 {
			log.Print(insts)
			return
		}
		id, ip := "ipv4", "8.8.8.8"
		log.Print("no instances found creating one")
		if _, err := w.cloudmap.RegisterInstance(&servicediscovery.RegisterInstanceInput{
			InstanceId: &id,
			ServiceId:  svcs.Services[0].Id,
			Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ip},
		}); err != nil {
			log.Printf("error registering instance: %v", err)
		}
		time.Sleep(time.Second * 10)
	}
}
