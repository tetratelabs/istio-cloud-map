package cloudmap

import (
	"testing"

	"istio.io/api/networking/v1alpha3"
)

func Test_store(t *testing.T) {
	t.Run("Store is read only", func(t *testing.T) {
		in := map[string][]*v1alpha3.ServiceEntry_Endpoint{"tetrate.io": []*v1alpha3.ServiceEntry_Endpoint{
			&v1alpha3.ServiceEntry_Endpoint{Address: "1.1.1.1", Ports: map[string]uint32{"http": 80}},
		}}
		st := NewStore()
		st.(*store).set(in)
		in["tetrate"] = []*v1alpha3.ServiceEntry_Endpoint{
			&v1alpha3.ServiceEntry_Endpoint{Address: "8.8.8.8", Ports: map[string]uint32{"http": 80}},
		}
		if st.Hosts()["tetrate.io"][0].Address == "8.8.8.8" {
			t.Errorf("We were able to affect the original input: %v", st.Hosts())
		}
	})
}
