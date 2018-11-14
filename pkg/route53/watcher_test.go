package route53

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go/service/servicediscovery"
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
var ipv41, ipv42, subdomain, hostname, portStr = "8.8.8.8", "9.9.9.9", "demo", "tetrate.io", "9999"

// golden path responses
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
		want        map[string][]endpoint
	}{
		{
			name:        "cache gets updated",
			listNsRes:   goldenPathListNamespaces,
			listSvcRes:  goldenPathListServices,
			discInstRes: goldenPathDiscoverInstances,
			want:        map[string][]endpoint{"demo.tetrate.io": []endpoint{endpoint{ipv41, 80}, endpoint{ipv41, 443}}},
		},
		{
			name:      "cache unchanged on ListNamespace error",
			listNsErr: errors.New("bang"),
			want:      nil,
		},
		{
			name:       "cache unchanged on ListService error",
			listNsRes:  goldenPathListNamespaces,
			listSvcErr: errors.New("bang"),
			want:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSDAPI{
				ListNsResult: tt.listNsRes, ListNsErr: tt.listNsErr,
				ListSvcResult: tt.listSvcRes, ListSvcErr: tt.listSvcErr,
				DiscInstResult: tt.discInstRes, DiscInstErr: tt.discInstErr,
			}
			w := &Watcher{cloudMapAPI: mockAPI, route53API: mockAPI}
			w.refreshCache()
			if !reflect.DeepEqual(w.hostCache, tt.want) {
				t.Errorf("Watcher.hostCache() = %v, want %v", w.hostCache, tt.want)
			}
		})
	}
}

func TestWatcher_hostsForNamespace(t *testing.T) {
	tests := []struct {
		name        string
		want        map[string][]endpoint
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
			want:        map[string][]endpoint{"demo.tetrate.io": []endpoint{endpoint{ipv41, 80}, endpoint{ipv41, 443}}},
		},
		{
			name:       "returns no hosts if host exists but has no endpoints",
			ns:         &servicediscovery.NamespaceSummary{Id: &hostname, Name: &hostname},
			listSvcRes: goldenPathListServices,
			discInstRes: &servicediscovery.DiscoverInstancesOutput{
				Instances: []*servicediscovery.HttpInstanceSummary{},
			},
			want: map[string][]endpoint{},
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
			w := &Watcher{cloudMapAPI: mockAPI, route53API: mockAPI}
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

func TestWatcher_endpointsForService(t *testing.T) {
	tests := []struct {
		name        string
		svc         *servicediscovery.ServiceSummary
		ns          *servicediscovery.NamespaceSummary
		discInstRes *servicediscovery.DiscoverInstancesOutput
		discInstErr error
		want        []endpoint
		wantErr     bool
	}{
		{
			name:        "Returns endpoints for service",
			discInstRes: goldenPathDiscoverInstances,
			svc:         &servicediscovery.ServiceSummary{Name: &subdomain},
			ns:          &servicediscovery.NamespaceSummary{Name: &hostname},
			want:        []endpoint{endpoint{ipv41, 80}, endpoint{ipv41, 443}},
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
			w := &Watcher{cloudMapAPI: mockAPI}
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
		want      []endpoint
	}{
		{
			name: "Handles multiple instances of the same type",
			instances: []*servicediscovery.HttpInstanceSummary{
				&servicediscovery.HttpInstanceSummary{Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41}},
				&servicediscovery.HttpInstanceSummary{Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv42}},
			},
			want: []endpoint{
				endpoint{ipv41, 80}, endpoint{ipv41, 443},
				endpoint{ipv42, 80}, endpoint{ipv42, 443},
			},
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
			want: []endpoint{endpoint{ipv41, 80}, endpoint{ipv41, 443}},
		},
		{
			name:      "Handles empty instances slice",
			instances: []*servicediscovery.HttpInstanceSummary{},
			want:      []endpoint{},
		},
		{
			name:      "Handles nil instances slice",
			instances: nil,
			want:      []endpoint{},
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
	port, _ := strconv.Atoi(portStr)
	tests := []struct {
		name     string
		instance *servicediscovery.HttpInstanceSummary
		want     []endpoint
	}{
		{
			name: "Single endpoint from AWS_INSTANCE_IPV4 instance with AWS_INSTANCE_PORT set",
			instance: &servicediscovery.HttpInstanceSummary{
				Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41, "AWS_INSTANCE_PORT": &portStr},
			},
			want: []endpoint{endpoint{ipv41, port}},
		},
		{
			name: "Two endpoints for http and https from AWS_INSTANCE_IPV4 instance without a port",
			instance: &servicediscovery.HttpInstanceSummary{
				Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41},
			},
			want: []endpoint{endpoint{ipv41, 80}, endpoint{ipv41, 443}},
		},
		{
			name: "Two endpoints for http and https from AWS_INSTANCE_IPV4 instance with non-int port",
			instance: &servicediscovery.HttpInstanceSummary{
				Attributes: map[string]*string{"AWS_INSTANCE_IPV4": &ipv41, "AWS_INSTANCE_PORT": &hostname},
			},
			want: []endpoint{endpoint{ipv41, 80}, endpoint{ipv41, 443}},
		},
		{
			name: "No endpoints for instance with AWS_ALIAS_DNS_NAME",
			instance: &servicediscovery.HttpInstanceSummary{
				InstanceId: &subdomain, ServiceName: &subdomain, NamespaceName: &hostname,
				Attributes: map[string]*string{"AWS_ALIAS_DNS_NAME": &hostname},
			},
			want: []endpoint{},
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
