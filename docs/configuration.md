# K8s Source Plugin Configuration Reference

The K8s source plugin connects to a Kubernetes cluster, fetches resources and loads it into any supported CloudQuery destination (e.g. PostgreSQL, BigQuery, Snowflake, and [more](/docs/plugins/destinations/overview)).

## Example

This example connects a single k8s context to a Postgres destination. The (top level) source spec section is described in the [Source Spec Reference](/docs/reference/source-spec).

:configuration

## K8s Spec

This is the (nested) spec used by K8s Source Plugin

- `contexts` (`[]string`) (optional) (default: empty. Will use the default context from K8s's config file)

  Specify K8s contexts to connect to.
  Specifying `*` will connect to all contexts available in the K8s config file (usually `~/.kube/config`).

- `concurrency` (`integer`) (optional) (default: `50000`):

  The best effort maximum number of Go routines to use.
  Lower this number to reduce memory usage.

- `concurrency_config` (`object`) (optional): Fine-tune concurrent Custom Resource fetching

  Configure how the plugin fetches multiple GVKs concurrently:

  - `max_concurrent_gvks` (`integer`) (optional) (default: `10`)

    Maximum number of Custom Resource GVKs to fetch in parallel. Values:
    - Default (10): Good for small-medium clusters with typical latency
    - Lower (2-5): For high-resource-constrained environments
    - Higher (20-50): For large clusters with many fast GVKs
    
    The plugin automatically scales this based on cluster size (1 worker per 50 nodes, capped at 50).

  - `fetch_timeout` (`duration`) (optional) (default: `"5m"`)

    Maximum time to fetch all resources for a single GVK. Examples: `"1m"`, `"5m"`, `"10m"`.
    
    Increase if your cluster has large custom resource collections or slower API servers.

  - `retry_attempts` (`integer`) (optional) (default: `3`)

    Number of retry attempts for transient errors (timeouts, rate limits, 500/502/503/504 errors).
    
    Does not retry permanent errors like RBAC denials or resource not found (404).

  - `retry_backoff` (`duration`) (optional) (default: `"100ms"`)

    Initial backoff duration before retrying. Exponentially increases after each attempt.
    
    Example: `"100ms"` → `"200ms"` → `"400ms"` → `"800ms"` (capped at `max_retry_backoff`)

  - `max_retry_backoff` (`duration`) (optional) (default: `"10s"`)

    Maximum backoff duration between retries. Prevents backoff from growing indefinitely.
    
    Backoff sequence caps at this value regardless of exponential growth.

- `custom_resources` (`[]object`) (optional): List of Custom Resources to collect

  Configure arbitrary Kubernetes Custom Resources by Group/Version/Kind:

  ```yaml
  custom_resources:
    - gvk: "cert-manager.io/v1/Certificate"
      namespaces: ["default", "cert-manager"]  # Optional: specific namespaces only
    - gvk: "argoproj.io/v1alpha1/Application"  # Optional: omit namespaces for cluster-wide
  ```

  Each custom resource configuration:

  - `gvk` (`string`) (required): Custom Resource identifier in format `"group/version/kind"`

    Examples:
    - `"cert-manager.io/v1/Certificate"` (has dots in group)
    - `"argoproj.io/v1alpha1/Application"`
    - `"networking.istio.io/v1beta1/VirtualService"`

  - `namespaces` (`[]string`) (optional): Limit collection to specific namespaces

    Omit this field to collect from all namespaces (cluster-wide).
    Specify array of namespace names for selective collection.
