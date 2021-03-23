package control

import (
	"context"
	"istio.io/api/meta/v1alpha1"
	"testing"

	"istio.io/api/networking/v1alpha3"
	icapi "istio.io/client-go/pkg/apis/networking/v1alpha3"
	ic "istio.io/client-go/pkg/clientset/versioned/typed/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tetratelabs/istio-cloud-map/pkg/control/mock"
	"github.com/tetratelabs/istio-cloud-map/pkg/infer"
)

var defaultHost = "tetrate.io"

var defaultEndpoints = []*v1alpha3.WorkloadEntry{
	{
		Address: "8.8.8.8",
		Ports:   map[string]uint32{"http": 80, "https": 443},
	},
}

var defaultHosts = map[string][]*v1alpha3.WorkloadEntry{
	defaultHost: defaultEndpoints,
}

var defaultServiceEntries = map[string]*icapi.ServiceEntry{
	defaultHost: {
		v1.TypeMeta{},
		v1.ObjectMeta{
			Name: infer.ServiceEntryName("cloud-map", defaultHost),
		},
		v1alpha3.ServiceEntry{
			Hosts: []string{defaultHost},
			// assume external for now
			Location:   v1alpha3.ServiceEntry_MESH_EXTERNAL,
			Resolution: infer.Resolution(defaultEndpoints),
			Ports:      infer.Ports(defaultEndpoints),
			Endpoints:  defaultEndpoints,
		},
		v1alpha1.IstioStatus{},
	},
}

func TestSynchronizer_garbageCollect(t *testing.T) {
	tests := []struct {
		name           string
		deleteCall     bool
		wantHost       string
		wantNamespace  string
		cloudMapHosts  map[string][]*v1alpha3.WorkloadEntry
		serviceEntries map[string]*icapi.ServiceEntry
	}{
		{
			name:           "Deletes Service Entry if host is no longer in Cloud Map",
			deleteCall:     true,
			serviceEntries: defaultServiceEntries,
			cloudMapHosts:  map[string][]*v1alpha3.WorkloadEntry{},
			wantHost:       "cloudmap-tetrate.io",
			wantNamespace:  "default",
		},
		{
			name:           "Keeps Service Entry if host is still in Cloud Map",
			deleteCall:     false,
			serviceEntries: defaultServiceEntries,
			cloudMapHosts:  defaultHosts,
		},
	}
	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &synchronizer{
				store:        &mock.Store{Result: tt.cloudMapHosts},
				serviceEntry: &mock.SEStore{Result: tt.serviceEntries},
				client:       &mockIstio{store: make(map[string]*icapi.ServiceEntry)},
			}
			s.garbageCollect(ctx)
			if s.client.(*mockIstio).DeleteCall != tt.deleteCall {
				t.Errorf("Delete called = %v, want %v", s.client.(*mockIstio).DeleteCall, tt.deleteCall)
			}
		})
	}
}

func TestSynchronizer_createOrUpdate(t *testing.T) {
	tests := []struct {
		name                            string
		host                            string
		createCall, updateCall, getCall bool
		cloudMapHosts                   map[string][]*v1alpha3.WorkloadEntry
		serviceEntries                  map[string]*icapi.ServiceEntry
		endpoints                       []*v1alpha3.WorkloadEntry
	}{
		{
			name:           "Does nothing if identical service entry exists",
			host:           defaultHost,
			cloudMapHosts:  defaultHosts,
			serviceEntries: defaultServiceEntries,
			endpoints:      defaultEndpoints,
		},
		{
			name:           "Updates Service Entry if new endpoints are added",
			getCall:        true,
			updateCall:     true,
			host:           defaultHost,
			cloudMapHosts:  defaultHosts,
			serviceEntries: defaultServiceEntries,
			endpoints: []*v1alpha3.WorkloadEntry{
				{
					Address: "8.8.8.8",
					Ports:   map[string]uint32{"http": 80, "https": 443},
								},
				{
					Address: "1.1.1.1",
					Ports:   map[string]uint32{"http": 80, "https": 443},
								},
			},
		},
		{
			name:           "Updates Service Entry if endpoints are removed",
			getCall:        true,
			updateCall:     true,
			host:           defaultHost,
			cloudMapHosts:  defaultHosts,
			serviceEntries: defaultServiceEntries,
			endpoints:      []*v1alpha3.WorkloadEntry{},
		},
		{
			name:           "Creates a new Service Entry if on doesn't exist",
			createCall:     true,
			host:           "not.tetrate.io",
			cloudMapHosts:  defaultHosts,
			serviceEntries: defaultServiceEntries,
			endpoints:      defaultEndpoints,
		},
	}
	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &synchronizer{
				store:        &mock.Store{Result: tt.cloudMapHosts},
				serviceEntry: &mock.SEStore{Result: tt.serviceEntries},
				client:       &mockIstio{store: make(map[string]*icapi.ServiceEntry)},
			}
			s.createOrUpdate(ctx, tt.host, tt.endpoints)
			if s.client.(*mockIstio).UpdateCall != tt.updateCall {
				t.Errorf("Update called = %v, want %v", s.client.(*mockIstio).UpdateCall, tt.createCall)
			}
			if s.client.(*mockIstio).GetCall != tt.getCall {
				t.Errorf("Get called = %v, want %v", s.client.(*mockIstio).GetCall, tt.getCall)
			}
			if s.client.(*mockIstio).CreateCall != tt.createCall {
				t.Errorf("Create called = %v, want %v", s.client.(*mockIstio).CreateCall, tt.createCall)
			}
		})
	}
}

type mockIstio struct {
	ic.ServiceEntryInterface

	store map[string]*icapi.ServiceEntry

	DeleteCall bool
	CreateCall bool
	UpdateCall bool
	GetCall    bool
}

func (mi *mockIstio) Delete(_ context.Context, _ string, _ v1.DeleteOptions) error {
	mi.DeleteCall = true
	return nil
}
func (mi *mockIstio) Create(_ context.Context, se *icapi.ServiceEntry, _ v1.CreateOptions) (*icapi.ServiceEntry, error) {
	mi.CreateCall = true
	mi.store[se.Name] = se
	return se, nil
}
func (mi *mockIstio) Update(_ context.Context, se *icapi.ServiceEntry, _ v1.UpdateOptions) (*icapi.ServiceEntry, error) {
	mi.UpdateCall = true
	mi.store[se.Name] = se
	return se, nil
}
func (mi *mockIstio) Get(_ context.Context, name string, _ v1.GetOptions) (*icapi.ServiceEntry, error) {
	mi.GetCall = true
	out, found := mi.store[name]
	if !found {
		out = &icapi.ServiceEntry{}
	}
	return out, nil
}
