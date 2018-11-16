package route53

import (
	"testing"
)

func Test_store(t *testing.T) {
	t.Run("Store is read only", func(t *testing.T) {
		in := map[string][]endpoint{
			"tetrate.io": []endpoint{endpoint{"1.1.1.1", 53}},
		}
		st := New()
		st.set(in)
		in["tetrate"] = []endpoint{endpoint{"8.8.8.8", 53}}
		if st.Hosts()["tetrate.io"][0].host == "8.8.8.8" {
			t.Errorf("We were able to affect the original input: %v", st.Hosts())
		}
	})
}
