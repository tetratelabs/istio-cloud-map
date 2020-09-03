package consul

import (
	"testing"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/tetratelabs/istio-cloud-map/pkg/provider"
)

func TestWatcher(t *testing.T) {
	conf := api.DefaultConfig()
	conf.WaitTime = time.Second
	client, err := api.NewClient(conf)
	if err != nil {
		t.Fatal(err)
	}

	// In order to have empty Consul for each test,
	// we sequentially execute tests.
	t.Run("listServices", func(t *testing.T) {
		testListServices(t, client)
		checkConsulEmpty(t, client)
	})

	t.Run("describeService", func(t *testing.T) {
		testDescribeService(t, client)
		checkConsulEmpty(t, client)
	})

	t.Run("refreshStore", func(t *testing.T) {
		testRefreshStore(t, client)
		checkConsulEmpty(t, client)
	})
}

func checkConsulEmpty(t *testing.T, client *api.Client) {
	w := NewWatcher(provider.NewStore(), client).(*watcher)
	if n, err := w.listServices(); err != nil {
		t.Fatalf("listServices failed: %v", err)
	} else if len(n) != 1 {
		t.Fatalf("service must be empty")
	}
}

func testRefreshStore(t *testing.T, client *api.Client) {
	tests := []struct {
		name     string
		services map[string][]*api.CatalogRegistration
	}{
		{
			name: "single",
			services: map[string][]*api.CatalogRegistration{
				"service1": {
					{
						ID:      "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
						Node:    "node1",
						Address: "1.1.1.1",
						Service: &api.AgentService{
							Service: "service1",
							Port:    8080,
							ID:      "service1",
						},
					},
				},
			},
		},
		{
			name: "multiple",
			services: map[string][]*api.CatalogRegistration{
				"service1": {
					{
						ID:      "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
						Node:    "node1",
						Address: "1.1.1.1",
						Service: &api.AgentService{
							Service: "service1",
							Port:    8080,
							ID:      "service1",
						},
					},
					{
						ID:      "baaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
						Node:    "node2",
						Address: "2.2.2.2",
						Service: &api.AgentService{
							Service: "service1",
							Port:    8080,
							ID:      "service1",
						},
					},
				},
				"service2": {{
					ID:      "caaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
					Node:    "node3",
					Address: "3.3.3.3",
					Service: &api.AgentService{
						Service: "service2",
						Port:    8080,
						ID:      "service2",
					},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, eps := range tt.services {
				for _, ep := range eps {
					_, err := client.Catalog().Register(ep, nil)
					if err != nil {
						t.Fatal(err)
					}
				}
			}

			defer func() {
				// clean up
				for _, eps := range tt.services {
					for _, ep := range eps {
						if _, err := client.Catalog().Deregister(&api.CatalogDeregistration{
							Node:      ep.Node,
							Address:   ep.Address,
							ServiceID: ep.Service.ID,
						}, nil); err != nil {
							t.Fatalf("failed to clean up service %v: %v", *ep, err)
						}
					}
				}
			}()

			w := NewWatcher(provider.NewStore(), client).(*watcher)
			w.refreshStore()

			actual := w.store.Hosts()
			if len(actual) != len(tt.services)+1 {
				t.Fatalf("number of hosts must be %d but got %d: %v", len(tt.services)+1, len(actual), actual)
			}

			for name, eps := range tt.services {
				actual := actual[name]
				if len(actual) != len(eps) {
					t.Fatalf("%s must have %d endpoints but got %d", name, len(eps), len(actual))
				}

				for _, exp := range eps {
					var found bool
					for _, e := range actual {
						if e.Address == exp.Address {
							found = true
							break
						}
					}

					if !found {
						t.Fatalf("address %s must exist as an endpoint of service %s", exp.Address, name)
					}
				}
			}

			prevIndex := w.lastIndex
			w.refreshStore() // supposed to immediately return since the index not change
			if prevIndex != w.lastIndex {
				t.Fatalf("indexes must not change but have %d != %d", prevIndex, w.lastIndex)
			}
		})
	}
}

func testDescribeService(t *testing.T, client *api.Client) {
	tests := []struct {
		name string
		sc   *api.CatalogRegistration
	}{
		{
			name: "found",
			sc: &api.CatalogRegistration{
				ID:      "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
				Node:    "node",
				Address: "1.1.1.1",
				Service: &api.AgentService{
					Service: "service1",
					Port:    8080,
					ID:      "service1",
				},
			},
		},
		{name: "not found", sc: &api.CatalogRegistration{Service: &api.AgentService{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sc.Service.Service != "" {
				_, err := client.Catalog().Register(tt.sc, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					// clean up
					if _, err := client.Catalog().Deregister(&api.CatalogDeregistration{
						Node:      tt.sc.Node,
						Address:   tt.sc.Address,
						ServiceID: tt.sc.Service.ID,
					}, nil); err != nil {
						t.Fatalf("failed to clean up service %v: %v", *tt.sc, err)
					}
				}()
			}

			w := NewWatcher(provider.NewStore(), client).(*watcher)
			ret, err := w.describeService(tt.sc.Service.Service)
			if tt.sc.Service.Service != "" {
				if err != nil {
					t.Fatal(err)
				}
				if len(ret) != 1 {
					t.Fatalf("the number of endpoint must be 1 but got %d", len(ret))
				}

				actual := ret[0]
				if actual.Address != tt.sc.Address {
					t.Fatalf("the returned address must be %s but got %s", tt.sc.Address, actual.Address)
				}
			} else if err == nil {
				t.Fatalf("err must be returned: %v", err)
			}
		})
	}
}

func testListServices(t *testing.T, client *api.Client) {
	tests := []struct {
		name     string
		services []*api.AgentServiceRegistration
	}{
		{name: "default"},
		{
			name: "non-default",
			services: []*api.AgentServiceRegistration{
				{Name: "1", ID: "1"}, {Name: "2", ID: "2"}, {Name: "3", ID: "3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, c := range tt.services {
				err := client.Agent().ServiceRegister(c)
				if err != nil {
					t.Fatal(err)
				}
			}
			defer func() {
				// clean up
				for _, s := range tt.services {
					if err := client.Agent().ServiceDeregister(s.ID); err != nil {
						t.Fatalf("failed to clean up service %s: %v", s.ID, err)
					}
				}
			}()

			w := NewWatcher(provider.NewStore(), client).(*watcher)
			actual, err := w.listServices()
			if err != nil {
				t.Fatal(err)
			}
			if len(actual) != len(tt.services)+1 { // `consul` service is registered by default
				t.Fatalf("the number of listed services must be %d + 1 but got %d: %v", len(tt.services), len(actual), actual)
			}

			for _, exp := range tt.services {
				if _, ok := actual[exp.Name]; !ok {
					t.Fatalf("%s must exist in the result", exp.Name)
				}
			}
		})
	}

	t.Run("timeout", func(t *testing.T) {
		s := tests[1].services[0]
		err := client.Agent().ServiceRegister(s)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			// clean up
			if err := client.Agent().ServiceDeregister(s.ID); err != nil {
				t.Fatalf("failed to clean up service %s: %v", s.ID, err)
			}
		}()

		w := NewWatcher(provider.NewStore(), client).(*watcher)
		_, err = w.listServices()
		if err != nil {
			t.Fatal(err)
		}

		_, err = w.listServices()
		if err != errIndexChangeTimeout {
			t.Fatalf(
				"`%v` must be returned but got `%v`",
				errIndexChangeTimeout, err,
			)
		}
	})
}

func TestCatalogServiceToEndpoints(t *testing.T) {
	// empty address
	res := catalogServiceToEndpoints(&api.CatalogService{})
	if res != nil {
		t.Errorf("result must be nil but got %v", res)
	}

	// empty port
	in := &api.CatalogService{Address: "1.2.3.4"}
	res = catalogServiceToEndpoints(in)
	if res.Address != in.Address {
		t.Errorf("address must be %s but got %s", in.Address, res.Address)
	}
	if res.Ports["http"] != 80 {
		t.Error("port 80 must be configured")
	}
	if res.Ports["https"] != 443 {
		t.Error("port 433 must be configured")
	}

	// address and ports are provided
	in = &api.CatalogService{Address: "1.2.3.4", ServicePort: 8080}
	res = catalogServiceToEndpoints(in)
	if res.Address != in.Address {
		t.Errorf("address must be %s but got %s", in.Address, res.Address)
	}
	if res.Ports["tcp"] != uint32(in.ServicePort) {
		t.Errorf("port %d must be of name tcp", in.ServicePort)
	}
}
