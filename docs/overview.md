The K8s Source plugin for CloudQuery extracts configuration from a variety of K8s APIs, including support for arbitrary Custom Resources with high-performance concurrent processing.

## Key Features

- **Custom Resource Collection**: Collect arbitrary Kubernetes Custom Resources by specifying Group/Version/Kind (GVK)
- **High-Performance Processing**: 5-10x faster syncs for multi-GVK configurations through concurrent worker pools with bounded concurrency
- **Resilient Error Handling**: Automatic retry for transient errors with exponential backoff; one GVK failure doesn't block others
- **Rich Observability**: Per-GVK timing metrics, throughput tracking, and sync summary with speedup factor
- **Namespace Filtering**: Optionally limit collection to specific namespaces
- **Dynamic Scaling**: Automatically adjusts concurrency based on cluster size
- **Thread-Safe**: Uses Kubernetes dynamic client for safe concurrent API calls

## Libraries in Use

- https://pkg.go.dev/k8s.io/api
- https://pkg.go.dev/golang.org/x/sync/errgroup (for concurrent processing)

## Authentication

:authentication