# K8s Source Plugin

The K8s Source plugin for CloudQuery extracts configuration from a variety of K8s APIs and loads it into any supported CloudQuery destination (e.g. PostgreSQL, BigQuery, Snowflake, and [more](https://www.cloudquery.io/docs/plugins/destinations/overview)).

## Features

### Custom Resources Support

The plugin supports collecting arbitrary Kubernetes Custom Resources (CRs) by specifying their Group, Version, and Kind (GVK). Custom resources are stored in the `k8s_custom_resources` table with their spec and status as JSON columns.

#### Configuration

Configure custom resources in your CloudQuery configuration file:

```yaml
kind: source
spec:
  name: k8s
  path: infobloxopen/k8s
  registry: github
  version: "v1.0.0"
  tables: ["*"]
  destinations: ["postgresql"]
  spec:
    contexts: ["my-cluster"]
    custom_resources:
      - gvk: "cert-manager.io/v1/Certificate"
        namespaces: ["default", "production"]
      - gvk: "argoproj.io/v1alpha1/Application"
      - gvk: "networking.istio.io/v1beta1/VirtualService"
```

**GVK Format**: `"group/version/kind"` (e.g., `"apps/v1/Deployment"`)
- Group can include dots (e.g., `cert-manager.io`, `networking.istio.io`)
- Version must be lowercase alphanumeric (e.g., `v1`, `v1beta1`, `v2alpha1`)
- Kind must be PascalCase starting with uppercase letter

**Namespace Filtering** (optional):
- Omit `namespaces` to collect from all namespaces
- Specify array to limit collection to specific namespaces

#### Common Custom Resources

**cert-manager** (Certificate Management):
```yaml
custom_resources:
  - gvk: "cert-manager.io/v1/Certificate"
  - gvk: "cert-manager.io/v1/CertificateRequest"
  - gvk: "cert-manager.io/v1/Issuer"
```

**ArgoCD** (GitOps):
```yaml
custom_resources:
  - gvk: "argoproj.io/v1alpha1/Application"
  - gvk: "argoproj.io/v1alpha1/AppProject"
```

**Istio** (Service Mesh):
```yaml
custom_resources:
  - gvk: "networking.istio.io/v1beta1/VirtualService"
  - gvk: "networking.istio.io/v1beta1/DestinationRule"
```

**Prometheus Operator** (Monitoring):
```yaml
custom_resources:
  - gvk: "monitoring.coreos.com/v1/ServiceMonitor"
  - gvk: "monitoring.coreos.com/v1/PrometheusRule"
```

#### Table Schema

See [k8s_custom_resources table documentation](docs/tables/k8s_custom_resources.md) for detailed column definitions, SQL query examples, and troubleshooting.

#### RBAC Requirements

Ensure your service account has permissions to list custom resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cloudquery-custom-resources
rules:
  - apiGroups: ["cert-manager.io"]
    resources: ["certificates", "issuers"]
    verbs: ["get", "list"]
  - apiGroups: ["argoproj.io"]
    resources: ["applications"]
    verbs: ["get", "list"]
```

# LICENSE

The code in this repo is copied from https://github.com/cloudquery/cloudquery
before being removed. in commit 7c93098e155cb94cd4c4c330490ad973af89ee3c

see https://github.com/cloudquery/cloudquery/commit/7c93098e155cb94cd4c4c330490ad973af89ee3c

Last commit 9bcb0bd4c0d1155f8e8492e425d150c793b9eb3d incorporated. This fork is a filter-diff of the plugins/source/k8s subdirectory.

