package convertors

import (
	"cape-project.eu/provider/pulumi/internal/schemas"
)

func ConvertPortToInt(in schemas.Port) int {
	return int(in)
}

func ConvertIntToPort(in int) schemas.Port {
	return schemas.Port(in)
}

func ConvertNetworkLoadBalancerPortToInt(in schemas.NetworkLoadBalancerPort) int {
	return int(in)
}

func ConvertIntToNetworkLoadBalancerPort(in int) schemas.NetworkLoadBalancerPort {
	return schemas.NetworkLoadBalancerPort(in)
}
