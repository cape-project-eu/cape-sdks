package convertors

import (
	"cape-project.eu/provider/pulumi/internal/schemas"
	"cape-project.eu/provider/pulumi/secapi/models"
)

// goverter:variables
// goverter:output:format assign-variable
// goverter:useZeroValueOnPointerInconsistency
var (
	ConvertReferenceURNToOpenAPI func(schemas.ReferenceURN) models.ReferenceURN
	ConvertReferenceToOpenAPI    func(schemas.Reference) models.Reference
	ConvertReferenceURNToPulumi  func(models.ReferenceURN) schemas.ReferenceURN
	ConvertReferenceToPulumi     func(models.Reference) schemas.Reference
)
