package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/krateoplatformops/plumbing/endpoints"
	"github.com/krateoplatformops/plumbing/http/request"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

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

func LoadInputData(contract ContractSpec) ([][]float32, error) {
	var inputTemp KrateoStorage
	if err := json.Unmarshal(contract.Input["krateo"].Raw, &inputTemp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal krateo storage: %w", err)
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("could not get inClusterConfig: %v", err)
	}
	endpoint, err := endpoints.FromSecret(context.Background(), cfg, inputTemp.Api.EndpointRef.Name, inputTemp.Api.EndpointRef.Namespace)
	if err != nil {
		return nil, fmt.Errorf("could not get endpoint secret: %v", err)
	}

	input := request.RequestOptions{
		RequestInfo: request.RequestInfo{
			Path:    inputTemp.Api.Path,
			Verb:    &inputTemp.Api.Verb,
			Payload: &inputTemp.Api.Payload,
			Headers: inputTemp.Api.Headers,
		},
		Endpoint: &endpoint,
	}

	var bodyData []byte
	input.ResponseHandler = func(rc io.ReadCloser) error {
		bodyData, _ = io.ReadAll(rc)
		return nil
	}

	toSend := map[string]any{}
	if contract.Parameters != nil {
		for k, v := range *contract.Parameters {
			toSend[k] = v
		}
	}

	payload, err := json.Marshal(toSend)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %v", err)
	}
	payloadString := string(payload)
	input.Payload = &payloadString

	res := request.Do(context.Background(), input)
	if res.Code < 200 || res.Code >= 300 {
		return nil, fmt.Errorf("failed to load input data, status: %s", res.Status)
	}

	var inputPayload map[string][][]float32
	err = json.Unmarshal(bodyData, &inputPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal input data: %w", err)
	}
	return inputPayload["result"], nil
}

func StoreOutputData(contract ContractSpec, toStore map[string][]float32) error {
	var outputTemp KrateoStorage
	if err := json.Unmarshal(contract.Output["krateo"].Raw, &outputTemp); err != nil {
		return fmt.Errorf("failed to unmarshal krateo storage: %w", err)
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("could not get inClusterConfig: %v", err)
	}
	endpoint, err := endpoints.FromSecret(context.Background(), cfg, outputTemp.Api.EndpointRef.Name, outputTemp.Api.EndpointRef.Namespace)
	if err != nil {
		return fmt.Errorf("could not get endpoint secret: %v", err)
	}

	output := request.RequestOptions{
		RequestInfo: request.RequestInfo{
			Path:    outputTemp.Api.Path,
			Verb:    &outputTemp.Api.Verb,
			Payload: &outputTemp.Api.Payload,
			Headers: outputTemp.Api.Headers,
		},
		Endpoint: &endpoint,
	}

	toSend := map[string]any{
		"job_uid": contract.JobId,
		"pod_uid": os.Getenv("pod_uid"),
	}
	if preds, ok := toStore["predictions"]; ok {
		b, err := json.Marshal(preds)
		if err != nil {
			return fmt.Errorf("failed to marshal predictions to string: %w", err)
		}
		toSend["predictions"] = string(b)
	} else {
		toSend["predictions"] = "[]"
	}
	if contract.Parameters != nil {
		for k, v := range *contract.Parameters {
			toSend[k] = v
		}
	}

	payload, err := json.Marshal(toSend)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}
	payloadString := string(payload)
	output.Payload = &payloadString

	res := request.Do(context.Background(), output)
	if res.Code < 200 || res.Code >= 300 {
		return fmt.Errorf("failed to store data, status code: %d", res.Code)
	}
	return nil
}

func runInferenceV2(contract ContractSpec, payload [][]float32) (map[string][]float32, error) {
	normalizedURL, err := normalizeURL(contract.KServe.ModelUrl)
	if err != nil {
		return nil, err
	}

	// Build V2 request
	requestBody := map[string]any{
		"inputs": []map[string]any{
			{
				"name":     contract.KServe.ModelInputName,
				"shape":    []int{len(payload), len(payload[0])},
				"datatype": "FP32",
				"data":     payload,
			},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		normalizedURL,
		bytes.NewBuffer(bodyBytes),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"kserve v2 inference failed: status=%d body=%s",
			resp.StatusCode,
			string(b),
		)
	}

	// Parse V2 response
	var v2Resp struct {
		Outputs []struct {
			Name string    `json:"name"`
			Data []float32 `json:"data"`
		} `json:"outputs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&v2Resp); err != nil {
		return nil, err
	}

	if len(v2Resp.Outputs) == 0 {
		return nil, fmt.Errorf("kserve v2 response has no outputs")
	}

	return map[string][]float32{
		"predictions": v2Resp.Outputs[0].Data,
	}, nil
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

	inputPayload, err := LoadInputData(contract)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load input data: %v\n", err)
		os.Exit(3)
	}

	result, err := runInferenceV2(contract, inputPayload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "inference error: %v\n", err)
		os.Exit(4)
	}

	if err := StoreOutputData(contract, result); err != nil {
		fmt.Fprintf(os.Stderr, "failed to store output: %v\n", err)
		os.Exit(5)
	}
	os.Exit(0)
}

func normalizeURL(raw string) (string, error) {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw, nil
	}
	return "http://" + raw, nil
}
