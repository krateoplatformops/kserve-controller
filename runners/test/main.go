package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"
)

type ContractSpec struct {
	JobId      string             `json:"jobId,omitempty"`
	JobName    string             `json:"jobName,omitempty"`
	KServe     KServeSpec         `json:"kserve"`
	Input      StorageMap         `json:"input,omitempty"`
	Output     StorageMap         `json:"output,omitempty"`
	Parameters *map[string]string `json:"parameters,omitempty"`
}

type KServeSpec struct {
	ModelName      string `json:"modelName,omitempty"`
	ModelUrl       string `json:"modelUrl,omitempty"`
	ModelVersion   string `json:"modelVersion,omitempty"`
	ModelInputName string `json:"modelInputName,omitempty"`
}

type StorageMap map[string]runtime.RawExtension

type KrateoStorage struct {
	Api finopsdatatypes.API `json:"api"`
}

func main() {
	contractPath := "/tmp/contract.json"

	contractBytes, err := os.ReadFile(contractPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read contract file: %v\n", err)
		os.Exit(2)
	}

	fmt.Fprintf(os.Stdout, "Contract: %s\n", string(contractBytes))

	var contract ContractSpec
	if err := json.Unmarshal(contractBytes, &contract); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse contract: %v\n", err)
		os.Exit(2)
	}

	if contract.KServe.ModelUrl == "" {
		fmt.Fprintln(os.Stderr, "kserve.url is required")
		os.Exit(2)
	}

	os.Exit(0)
}

func normalizeURL(raw string) (string, error) {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw, nil
	}
	return "http://" + raw, nil
}
