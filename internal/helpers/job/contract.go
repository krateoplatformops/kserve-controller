package job

import controllerapi "kserve-controller/api/v1"

// This file defines the contract structure for KServe inference jobs launched by InferenceRun resources.
// Note: the jobs themselves do not run the inference. Kserve jobs will run the inference.
// These jobs only retrieve input data, call KServe endpoints, and store the results.
type ContractSpec struct {
	JobId      string                   `json:"jobId,omitempty"`
	JobName    string                   `json:"jobName,omitempty"`
	KServe     controllerapi.KServeSpec `json:"kserve"`
	Input      controllerapi.StorageMap `json:"input,omitempty"`
	Output     controllerapi.StorageMap `json:"output,omitempty"`
	Parameters *map[string]string       `json:"parameters,omitempty"`
}
