package v1

import (
	"encoding/json"
	"fmt"
	"kserve-controller/internal/helpers/storage"
	"kserve-controller/internal/helpers/storage/providers"

	runtime "k8s.io/apimachinery/pkg/runtime"
)

type StorageMap map[storage.StorageLabel]runtime.RawExtension

func (r *InferenceConfig) GetStorageProvider() (storage.StorageInterface, storage.StorageInterface, error) {
	var input storage.StorageInterface
	var output storage.StorageInterface

	for spec, rawExt := range r.Spec.Storage.Input {
		switch spec {
		case storage.KrateoStorage:
			var input providers.KrateoStorage
			if err := json.Unmarshal(rawExt.Raw, &input); err != nil {
				return nil, nil, fmt.Errorf("failed to unmarshal krateo storage: %w", err)
			}
		// Add other storage providers here
		default:
			input = rawExt
		}
	}

	for spec, rawExt := range r.Spec.Storage.Output {
		switch spec {
		case storage.KrateoStorage:
			var output providers.KrateoStorage
			if err := json.Unmarshal(rawExt.Raw, &output); err != nil {
				return nil, nil, fmt.Errorf("failed to unmarshal krateo storage: %w", err)
			}
		// Add other storage providers here
		default:
			output = rawExt
		}
	}

	return input, output, nil

}
