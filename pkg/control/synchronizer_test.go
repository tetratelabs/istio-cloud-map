package control

import (
	"testing"

	cmMock "github.com/tetratelabs/istio-cloud-map/pkg/cloudmap/mock"
	"github.com/tetratelabs/istio-cloud-map/pkg/infer"
	"github.com/tetratelabs/istio-cloud-map/pkg/serviceentry"
	seMock "github.com/tetratelabs/istio-cloud-map/pkg/serviceentry/mock"

	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"istio.io/istio/pilot/pkg/model"
)

type mockIstio struct {
	*crd.Client

	DeleteCall bool
	CreateCall bool
	UpdateCall bool
	GetCall    bool
}

func (mi *mockIstio) Delete(_, _, _ string) error {
	mi.DeleteCall = true
	return nil
}
func (mi *mockIstio) Create(_ model.Config) (string, error) {
	mi.CreateCall = true
	return "", nil
}
func (mi *mockIstio) Update(_ model.Config) (string, error) {
	mi.UpdateCall = true
	return "", nil
}
func (mi *mockIstio) Get(_, _, _ string) (*model.Config, bool) {
	mi.GetCall = true
	return &model.Config{}, true
}

var defaultHost = "tetrate.io"

var defaultEndpoints = []*v1alpha3.ServiceEntry_Endpoint{
	&v1alpha3.ServiceEntry_Endpoint{
		Address: "8.8.8.8",
		Ports:   map[string]uint32{"http": 80, "https": 443},
	},
}

var defaultHosts = map[string][]*v1alpha3.ServiceEntry_Endpoint{
	defaultHost: defaultEndpoints,
}

var defaultServiceEntries = map[string]*serviceentry.Entry{
	defaultHost: &serviceentry.Entry{
		Spec: &v1alpha3.ServiceEntry{
			Hosts: []string{defaultHost},
			// assume external for now
			Location:   v1alpha3.ServiceEntry_MESH_EXTERNAL,
			Resolution: infer.Resolution(defaultEndpoints),
			Ports:      infer.Ports(defaultEndpoints),
			Endpoints:  defaultEndpoints,
		},
	},
}

func TestSynchronizer_garbageCollect(t *testing.T) {
	tests := []struct {
		name           string
		deleteCall     bool
		wantHost       string
		wantNamespace  string
		cloudMapHosts  map[string][]*v1alpha3.ServiceEntry_Endpoint
		serviceEntries map[string]*serviceentry.Entry
	}{
		{
			name:           "Deletes Service Entry if host is no longer in Cloud Map",
			deleteCall:     true,
			serviceEntries: defaultServiceEntries,
			cloudMapHosts:  map[string][]*v1alpha3.ServiceEntry_Endpoint{},
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &synchronizer{
				cloudMap:     &cmMock.Store{Result: tt.cloudMapHosts},
				serviceEntry: &seMock.Store{Result: tt.serviceEntries},
				istio:        &mockIstio{},
			}
			s.garbageCollect()
			if s.istio.(*mockIstio).DeleteCall != tt.deleteCall {
				t.Errorf("Delete called = %v, want %v", s.istio.(*mockIstio).DeleteCall, tt.deleteCall)
			}
		})
	}
}

func TestSynchronizer_createOrUpdate(t *testing.T) {
	tests := []struct {
		name                            string
		host                            string
		createCall, updateCall, getCall bool
		cloudMapHosts                   map[string][]*v1alpha3.ServiceEntry_Endpoint
		serviceEntries                  map[string]*serviceentry.Entry
		endpoints                       []*v1alpha3.ServiceEntry_Endpoint
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
			endpoints: []*v1alpha3.ServiceEntry_Endpoint{
				&v1alpha3.ServiceEntry_Endpoint{
					Address: "8.8.8.8",
					Ports:   map[string]uint32{"http": 80, "https": 443},
				},
				&v1alpha3.ServiceEntry_Endpoint{
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
			endpoints:      []*v1alpha3.ServiceEntry_Endpoint{},
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Synchronizer{
				cloudMap:     &cmMock.Store{Result: tt.cloudMapHosts},
				serviceEntry: &seMock.Store{Result: tt.serviceEntries},
				istio:        &mockIstio{},
			}
			s.createOrUpdate(tt.host, tt.endpoints)
			if s.istio.(*mockIstio).UpdateCall != tt.updateCall {
				t.Errorf("Update called = %v, want %v", s.istio.(*mockIstio).UpdateCall, tt.createCall)
			}
			if s.istio.(*mockIstio).GetCall != tt.getCall {
				t.Errorf("Get called = %v, want %v", s.istio.(*mockIstio).GetCall, tt.getCall)
			}
			if s.istio.(*mockIstio).CreateCall != tt.createCall {
				t.Errorf("Create called = %v, want %v", s.istio.(*mockIstio).CreateCall, tt.createCall)
			}
		})
	}
}
