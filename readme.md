# KServe Inference Controller

This repository contains the Kubernetes custom controller for managing long-running inferences on KServe. It decouples request handling from execution by leveraging job-based orchestration.

## Summary

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [The Runner Contract](#the-runner-contract)
4. [Examples](#examples)
5. [Configuration](#configuration)

## Overview

The **KServe Inference Controller** is designed to handle complex, long-running inference tasks that would typically time out or block a standard controller loop. Instead of making direct HTTP calls to KServe, the operator orchestrates **Kubernetes Jobs** to act as "Runners." These runners handle the entire lifecycle of an inference: data retrieval from a source, the inference call itself, and the persistence of results to a storage backend.

This approach ensures:

* **Resilience:** If a node fails, the Job is automatically rescheduled.
* **Scalability:** Each inference run has its own dedicated resources.
* **Extensibility:** Support for new data sources or storage backends only requires updating the runner image, not the core controller.

## Architecture

The system revolves around two primary Custom Resources (CRs) and a specialized Runner image:

* **InferenceConfig:** A static definition of a model's deployment. It specifies the KServe endpoint, the runner image to use, and the storage backend configuration (Input/Output).
* **InferenceRun:** A single execution trigger. It references a config and provides specific execution parameters (e.g., table names, query filters).
* **Runner Job:** A containerized process spawned by the controller that executes the business logic defined in the "Contract."

### Workflow

1. User creates an **InferenceConfig** for a specific model (e.g., `sklearn-iris`).
2. User triggers an **InferenceRun** referencing that config.
3. The **Controller** generates a **Contract** (JSON) and spawns a **Kubernetes Job**.
4. The **Runner** reads the contract, fetches data, calls KServe, and saves the output.
5. The **Controller** monitors the Job and updates the `InferenceRun` status.

## The Runner Contract

The communication between the controller and the execution Job is governed by a **Contract**. This contract is passed to the runner as a JSON file, allowing for a standardized way to handle diverse storage backends. The runner is meant to be as lightweight as possible and only get/store data. No computation should be done by the runner. Data transformations should be handled by a notebook in the [finops-database-handler](https://github.com/krateoplatformops/finops-database-handler).

Example contract passed by the controller to the Runner, through a ConfigMap:
```json
{
   "jobId":"5be07ada-5fe0-4c9e-a8bc-aaae67d8d344",
   "jobName":"inf-example-inference-run-triton-5be07ada",
   "kserve":{
      "modelName":"sklearn-iris",
      "modelUrl":"kserve-krateo-ttm-predictor.kserve-test.svc.cluster.local/v2/models/granite-timeseries-ttm-r2/infer",
      "modelVersion":"v2",
      "modelInputName":"past_values"
   },
   "input":{
      "krateo":{
         "api":{
            "endpointRef":{
               "name":"finops-database-handler-endpoint",
               "namespace":"kserve-controller-system"
            },
            "headers":[
               "Accept: application/json",
               "Content-Type: application/json"
            ],
            "path":"/compute/tritoninput",
            "verb":"POST"
         }
      }
   },
   "output":{
      "krateo":{
         "api":{
            "endpointRef":{
               "name":"finops-database-handler-endpoint",
               "namespace":"kserve-controller-system"
            },
            "headers":[
               "Accept: application/json",
               "Content-Type: application/json"
            ],
            "path":"/compute/tritonoutput",
            "verb":"POST"
         }
      }
   },
   "parameters":{
      "input_data_length":"512",
      "input_table_column_name":"average",
      "input_table_name":"azuretoolkit",
      "key_name":"resourceid",
      "key_value":"sample_vm",
      "output_table_name":"kserve_controller_output_triton"
   }
}
```
To see how this specific contract is used, check `runners/krateo/main.go` and the counter part notebooks in `charts/chart/templates/notebook-triton.yaml`.

### Extensibility via RawExtension

The `storage.input` and `storage.output` keys in the CRD have no schema. This allows the `InferenceConfig` to support any storage provider (e.g., Krateo FinOps, S3, GCS, etc.) without changing the controller. The runner receives the contract with the data unmodified. Therefore, by providing a specialized runner image, you can implement custom logic to parse these raw configurations and interact with any proprietary or cloud-native data store.

## Examples

### InferenceConfig

Defines the "How" and "Where" of the model and storage.

```yaml
apiVersion: ai.krateo.io/v1
kind: InferenceConfig
metadata:
  name: iris-inference-config
  namespace: kserve-controller-system
spec:
  autoDeletePolicy: None # None | DeleteOnCompletion | DeleteOnSuccess
  image: ghcr.io/krateoplatformops/kserve-krateo-runner:0.1.0
  modelName: sklearn-iris
  modelUrl: sklearn-iris-predictor.kserve-test.svc.cluster.local/v2/models/sklearn-iris/infer
  modelVersion: v2
  storage:
    input:
      krateo:
        api:
          endpointRef: 
            name: finops-database-handler-endpoint
            namespace: kserve-controller-system
          path: /compute/kserveinput
          verb: POST
          headers:
          - 'Accept: application/json'
          - 'Content-Type: application/json'
    output:
      krateo:
        api:
          endpointRef: 
            name: finops-database-handler-endpoint
            namespace: kserve-controller-system
          path: /compute/kserveoutput
          verb: POST
          headers:
          - 'Accept: application/json'
          - 'Content-Type: application/json'
```

### InferenceRun

Defines the "When" and "What" of a specific execution.

```yaml
apiVersion: ai.krateo.io/v1
kind: InferenceRun
metadata:
  name: iris-run-january
  namespace: kserve-controller-system
spec:
  configRef:
    name: example-inference-config
    namespace: kserve-controller-system
  timeoutSeconds: 3600
  parameters:
    input_table_name: kserve_controller_input
    output_table_name: kserve_controller_output

```
The parameters are passed as is in the JSON contract.

## Configuration

The controller can be configured via environment variables to tune its reconciliation behavior:

* **`POLLING_INTERVAL`**: Duration (e.g., `1m`) between status checks.
* **`MAX_RECONCILE_RATE`**: Number of concurrent workers.

### Installation
The operator can be installed through its Helm chart:
```sh
helm repo add krateo https://charts.krateo.io
helm repo update krateo
helm install kserve-controller krateo/kserve-controller
```

### Repository Structure
The repository contains several objects needed by the `kserve-controller`:
- the controller code in `internal` and `api`
- the helm charts for the controller and crd in `charts/chart` and `charts/crd-chart`
- the Krateo runner for the finops-database-handler in `runners/krateo`
- the model for TTM adapted for the Triton KServe engine in `models`
- example CRs in `testdata`