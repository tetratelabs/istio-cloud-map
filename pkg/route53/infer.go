package route53

import (
	"istio.io/api/networking/v1alpha3"
)

func inferEndpoint(address string, port uint32) *v1alpha3.ServiceEntry_Endpoint {
	return &v1alpha3.ServiceEntry_Endpoint{
		Address: address,
		Ports:   map[string]uint32{inferProto(port): port},
	}
}

func inferProto(port uint32) string {
	switch port {
	case 80:
		return "http"
	case 443:
		return "https"
	default:
		return "tcp"
	}
}
