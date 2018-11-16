package route53

import (
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"istio.io/api/networking/v1alpha3"
)

type mockSDAPI struct {
	servicediscovery.ServiceDiscovery

	ListNsResult   *servicediscovery.ListNamespacesOutput
	ListNsErr      error
	ListSvcResult  *servicediscovery.ListServicesOutput
	ListSvcErr     error
	DiscInstResult *servicediscovery.DiscoverInstancesOutput
	DiscInstErr    error
}

func (m *mockSDAPI) ListNamespaces(lni *servicediscovery.ListNamespacesInput) (
	*servicediscovery.ListNamespacesOutput, error) {
	return m.ListNsResult, m.ListNsErr
}
func (m *mockSDAPI) ListServices(lsi *servicediscovery.ListServicesInput) (
	*servicediscovery.ListServicesOutput, error) {
	filter := lsi.Filters[0]
	if filter.Condition != &filterConditionEquals || filter.Name != &serviceFilterNamespaceID {
		return nil, errors.New("Namespace ID filter is not present")
	}
	return m.ListSvcResult, m.ListSvcErr
}

func (m *mockSDAPI) DiscoverInstances(dii *servicediscovery.DiscoverInstancesInput) (
	*servicediscovery.DiscoverInstancesOutput, error) {
	if dii.ServiceName == nil {
		return nil, errors.New("Service name was not provided")
	}
	if dii.NamespaceName == nil {
		return nil, errors.New("Namespace name was not provided")
	}
	return m.DiscInstResult, m.DiscInstErr
}

// various strings to allow pointer usage
var ipv41, ipv42, subdomain, hostname, portStr, httpPortStr = "8.8.8.8", "9.9.9.9", "demo", "tetrate.io", "9999", "80"

// golden path responses
var inferedIPv41Endpoint = v1alpha3.ServiceEntry_Endpoint{Address: ipv41, Ports: map[string]uint32{"http": 80, "https": 443}}
var inferedIPv42Endpoint = v1alpha3.ServiceEntry_Endpoint{Address: ipv42, Ports: map[string]uint32{"http": 80, "https": 443}}

var goldenPathListNamespaces = &servicediscovery.ListNamespacesOutput{
	Namespaces: []*servicediscovery.NamespaceSummary{
		&servicediscovery.NamespaceSummary{Id: &hostname, Name: &hostname},
	},
}
var goldenPathListServices = &servicediscovery.ListServicesOutput{
	Services: []*servicediscovery.ServiceSummary{
		&servicediscovery.ServiceSummary{Name: &subdomain},
	},
}
var goldenPathDiscoverInstances = &servicediscovery.DiscoverInstancesOutput{
	Instances: []*servicediscovery.HttpInstanceSummary{
		&servicediscovery.HttpInstanceSummary{Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41}},
	},
}

func TestWatcher_refreshCache(t *testing.T) {
	tests := []struct {
		name        string
		listNsRes   *servicediscovery.ListNamespacesOutput
		listNsErr   error
		listSvcRes  *servicediscovery.ListServicesOutput
		listSvcErr  error
		discInstRes *servicediscovery.DiscoverInstancesOutput
		discInstErr error
		want        map[string][]v1alpha3.ServiceEntry_Endpoint
	}{
		{
			name:        "store gets updated",
			listNsRes:   goldenPathListNamespaces,
			listSvcRes:  goldenPathListServices,
			discInstRes: goldenPathDiscoverInstances,
			want:        map[string][]v1alpha3.ServiceEntry_Endpoint{"demo.tetrate.io": []v1alpha3.ServiceEntry_Endpoint{inferedIPv41Endpoint}},
		},
		{
			name:      "store unchanged on ListNamespace error",
			listNsErr: errors.New("bang"),
			want:      map[string][]v1alpha3.ServiceEntry_Endpoint{},
		},
		{
			name:       "store unchanged on ListService error",
			listNsRes:  goldenPathListNamespaces,
			listSvcErr: errors.New("bang"),
			want:       map[string][]v1alpha3.ServiceEntry_Endpoint{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSDAPI{
				ListNsResult: tt.listNsRes, ListNsErr: tt.listNsErr,
				ListSvcResult: tt.listSvcRes, ListSvcErr: tt.listSvcErr,
				DiscInstResult: tt.discInstRes, DiscInstErr: tt.discInstErr,
			}
			w := &Watcher{cloudmap: mockAPI, r53: mockAPI, store: NewStore()}
			w.refreshStore()
			if !reflect.DeepEqual(w.store.Hosts(), tt.want) {
				t.Errorf("Watcher.store = %v, want %v", w.store.Hosts(), tt.want)
			}
		})
	}
}

func TestWatcher_hostsForNamespace(t *testing.T) {
	tests := []struct {
		name        string
		want        map[string][]v1alpha3.ServiceEntry_Endpoint
		ns          *servicediscovery.NamespaceSummary
		listSvcRes  *servicediscovery.ListServicesOutput
		listSvcErr  error
		discInstRes *servicediscovery.DiscoverInstancesOutput
		discInstErr error
		wantErr     bool
	}{
		{
			name:        "returns hosts for the given namespace",
			ns:          &servicediscovery.NamespaceSummary{Id: &hostname, Name: &hostname},
			listSvcRes:  goldenPathListServices,
			discInstRes: goldenPathDiscoverInstances,
			want:        map[string][]v1alpha3.ServiceEntry_Endpoint{"demo.tetrate.io": []v1alpha3.ServiceEntry_Endpoint{inferedIPv41Endpoint}},
		},
		{
			name:       "returns empty host if host exists but has no Endpoints",
			ns:         &servicediscovery.NamespaceSummary{Id: &hostname, Name: &hostname},
			listSvcRes: goldenPathListServices,
			discInstRes: &servicediscovery.DiscoverInstancesOutput{
				Instances: []*servicediscovery.HttpInstanceSummary{},
			},
			want: map[string][]v1alpha3.ServiceEntry_Endpoint{"demo.tetrate.io": []v1alpha3.ServiceEntry_Endpoint{}},
		},
		{
			name:        "errors if DiscoverInstances errors",
			ns:          &servicediscovery.NamespaceSummary{Id: &hostname, Name: &hostname},
			listSvcRes:  goldenPathListServices,
			discInstErr: errors.New("bang"),
			wantErr:     true,
		},
		{
			name:       "errors if ListServices errors",
			ns:         &servicediscovery.NamespaceSummary{Id: &hostname, Name: &hostname},
			listSvcErr: errors.New("bang"),
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSDAPI{
				DiscInstResult: tt.discInstRes, DiscInstErr: tt.discInstErr,
				ListSvcResult: tt.listSvcRes, ListSvcErr: tt.listSvcErr,
			}
			w := &Watcher{cloudmap: mockAPI, r53: mockAPI}
			got, err := w.hostsForNamespace(tt.ns)
			if (err != nil) != tt.wantErr {
				t.Errorf("Watcher.hostsForNamespace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Watcher.hostsForNamespace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWatcher_EndpointsForService(t *testing.T) {
	tests := []struct {
		name        string
		svc         *servicediscovery.ServiceSummary
		ns          *servicediscovery.NamespaceSummary
		discInstRes *servicediscovery.DiscoverInstancesOutput
		discInstErr error
		want        []v1alpha3.ServiceEntry_Endpoint
		wantErr     bool
	}{
		{
			name:        "Returns Endpoints for service",
			discInstRes: goldenPathDiscoverInstances,
			svc:         &servicediscovery.ServiceSummary{Name: &subdomain},
			ns:          &servicediscovery.NamespaceSummary{Name: &hostname},
			want:        []v1alpha3.ServiceEntry_Endpoint{inferedIPv41Endpoint},
		},
		{
			name:        "Errors if call to DiscoverInstances errors",
			discInstErr: errors.New("bang"),
			svc:         &servicediscovery.ServiceSummary{Name: &subdomain},
			ns:          &servicediscovery.NamespaceSummary{Name: &hostname},
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSDAPI{DiscInstResult: tt.discInstRes, DiscInstErr: tt.discInstErr}
			w := &Watcher{cloudmap: mockAPI}
			got, err := w.endpointsForService(tt.svc, tt.ns)
			if (err != nil) != tt.wantErr {
				t.Errorf("Watcher.endpointsForService() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Watcher.endpointsForService() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_instancesToEndpoints(t *testing.T) {
	tests := []struct {
		name      string
		instances []*servicediscovery.HttpInstanceSummary
		want      []v1alpha3.ServiceEntry_Endpoint
	}{
		{
			name: "Handles multiple instances of the same type",
			instances: []*servicediscovery.HttpInstanceSummary{
				&servicediscovery.HttpInstanceSummary{Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41}},
				&servicediscovery.HttpInstanceSummary{Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv42}},
			},
			want: []v1alpha3.ServiceEntry_Endpoint{inferedIPv41Endpoint, inferedIPv42Endpoint},
		},
		{
			name: "Handles multiple instances of differing type",
			instances: []*servicediscovery.HttpInstanceSummary{
				&servicediscovery.HttpInstanceSummary{Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41}},
				&servicediscovery.HttpInstanceSummary{
					InstanceId: &subdomain, ServiceName: &subdomain, NamespaceName: &hostname,
					Attributes: map[string]*string{"AWS_ALIAS_DNS_NAME": &hostname},
				},
			},
			want: []v1alpha3.ServiceEntry_Endpoint{inferedIPv41Endpoint},
		},
		{
			name: "handles empty instance attributes map",
			instances: []*servicediscovery.HttpInstanceSummary{
				&servicediscovery.HttpInstanceSummary{
					InstanceId: &subdomain, ServiceName: &subdomain, NamespaceName: &hostname,
					Attributes: map[string]*string{},
				},
			},
			want: []v1alpha3.ServiceEntry_Endpoint{},
		},
		{
			name:      "Handles empty instances slice",
			instances: []*servicediscovery.HttpInstanceSummary{},
			want:      []v1alpha3.ServiceEntry_Endpoint{},
		},
		{
			name:      "Handles nil instances slice",
			instances: nil,
			want:      []v1alpha3.ServiceEntry_Endpoint{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := instancesToEndpoints(tt.instances); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("instancesToEndpoints() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_instanceToEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		instance *servicediscovery.HttpInstanceSummary
		want     *v1alpha3.ServiceEntry_Endpoint
	}{
		{
			name: "Endpoint from AWS_INSTANCE_IPV4 instance with AWS_INSTANCE_PORT set to known proto",
			instance: &servicediscovery.HttpInstanceSummary{
				Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41, "AWS_INSTANCE_PORT": &httpPortStr},
			},
			want: &v1alpha3.ServiceEntry_Endpoint{Address: ipv41, Ports: map[string]uint32{"http": 80}},
		},
		{
			name: "Endpoint from AWS_INSTANCE_IPV4 instance with AWS_INSTANCE_PORT set to unknown proto",
			instance: &servicediscovery.HttpInstanceSummary{
				Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41, "AWS_INSTANCE_PORT": &portStr},
			},
			want: &v1alpha3.ServiceEntry_Endpoint{Address: ipv41, Ports: map[string]uint32{"tcp": 9999}},
		},
		{
			name: "Endpoint infering http and https from AWS_INSTANCE_IPV4 instance without a port",
			instance: &servicediscovery.HttpInstanceSummary{
				Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41},
			},
			want: &inferedIPv41Endpoint,
		},
		{
			name: "Endpoint infering http and https from AWS_INSTANCE_IPV4 instance with non-int port",
			instance: &servicediscovery.HttpInstanceSummary{
				Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41, "AWS_INSTANCE_PORT": &hostname},
			},
			want: &inferedIPv41Endpoint,
		},
		{
			name: "Nil for instance with AWS_ALIAS_DNS_NAME",
			instance: &servicediscovery.HttpInstanceSummary{
				InstanceId: &subdomain, ServiceName: &subdomain, NamespaceName: &hostname,
				Attributes: map[string]*string{"AWS_ALIAS_DNS_NAME": &hostname},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := instanceToEndpoint(tt.instance); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("instanceToEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}
