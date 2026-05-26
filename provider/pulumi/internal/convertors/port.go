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
