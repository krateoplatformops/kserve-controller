# kserve-inference-controller

A Kubernetes controller that manages long-running KServe inference tasks by orchestrating dedicated Kubernetes Jobs as runners, decoupling request handling from execution for resilience and scalability.

📖 **Full documentation**: [docs.krateo.io — kserve-inference-controller](https://docs.krateo.io/key-concepts/kcf/finops-components/kserve-controller)

---

## Key features

- Orchestrates Kubernetes Jobs (and CronJobs for scheduled runs) as lightweight runners to handle the full inference lifecycle: data retrieval, KServe call, and result persistence
- Communicates with runners via a standardized JSON contract, enabling support for any storage backend without changing the core controller
- Extensible storage layer via schema-free `input`/`output` fields, allowing custom runner images to integrate with any data source or sink

## Requirements

| Dependency | Minimum version |
|------------|----------------|
| Kubernetes | v1.31 |
| Krateo | v3.0.0 |
| KServe | v0.15 |
| finops-database-handler | v0.5.3 |

## Install

```bash
helm repo add krateo https://charts.krateo.io
helm repo update
helm template kserve-controller krateo/kserve-controller | kubectl apply -f -
```

> For advanced installation options, custom values, and upgrade instructions, see the [installation guide](https://docs.krateo.io/key-concepts/kcf/finops-components/kserve-controller).

## Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `POLLING_INTERVAL` | No | — | Duration between Job status checks (e.g. `1m`) |
| `MAX_RECONCILE_RATE` | No | — | Number of concurrent reconcile workers |