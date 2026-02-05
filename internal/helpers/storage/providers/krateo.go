// This file handles connections to the finops-database-handler

package providers

import (
	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"
)

type KrateoStorage struct {
	Api finopsdatatypes.API `json:"api"`
}
