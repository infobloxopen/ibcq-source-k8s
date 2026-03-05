# Table: k8s_custom_resources

This table shows custom Kubernetes resources based on your configuration. Unlike core Kubernetes resources (pods, services, etc.) which have dedicated tables, this table allows you to sync any Custom Resource Definition (CRD) installed in your cluster.

## Primary Key

- `uid` (when write_mode is `overwrite` or `overwrite-delete-stale`)
- No primary key when write_mode is `append`

**Note**: When using `overwrite` or `overwrite-delete-stale` mode, a unique constraint is also created on `_cq_id`. The `_cq_id` is deterministically generated from the `uid` primary key.

## Columns

| Name | Type | Description |
| ---- | ---- | ----------- |
| _cq_id | UUID | CloudQuery internal ID (deterministic hash of uid; unique constraint when write_mode is overwrite or overwrite-delete-stale) |
| _cq_parent_id | UUID | CloudQuery internal parent ID |
| context | String | Kubernetes context name |
| api_version | String | API version (group/version, e.g., "cert-manager.io/v1") |
| kind | String | Resource kind (e.g., "Certificate") |
| namespace | String | Namespace of the resource. Empty for cluster-scoped resources |
| name | String | Name of the resource |
| uid | String | Unique identifier for the resource (primary key when write_mode is overwrite or overwrite-delete-stale) |
| labels | JSON | Resource labels (stored as jsonb in PostgreSQL) |
| annotations | JSON | Resource annotations (stored as jsonb in PostgreSQL) |
| owner_references | JSON | Owner references (stored as jsonb in PostgreSQL) |
| finalizers | List\<String\> | Resource finalizers |
| spec | JSON | Resource specification (stored as jsonb in PostgreSQL) |
| status | JSON | Resource status (stored as jsonb in PostgreSQL) |

## Configuration

To sync custom resources, add them to your configuration file:

```yaml
kind: source
spec:
  name: k8s
  path: cloudquery/k8s
  registry: cloudquery
  version: "v3.0.0"
  tables: ["k8s_custom_resources"]
  destinations: ["postgresql"]
  
  spec:
    contexts: ["*"]  # or specific contexts
    
    custom_resources:
      # Cert-manager Certificates from specific namespaces
      - gvk: "cert-manager.io/v1/Certificate"
        namespaces: ["default", "production"]
      
      # ArgoCD Applications (all namespaces)
      - gvk: "argoproj.io/v1alpha1/Application"
      
      # Cluster-scoped resource
      - gvk: "cert-manager.io/v1/ClusterIssuer"
```

### GVK Format

The `gvk` field must be in the format `"group/version/kind"`:
- **group**: API group (e.g., `cert-manager.io`, `argoproj.io`)
- **version**: API version (e.g., `v1`, `v1alpha1`, `v1beta1`)
- **kind**: Resource kind (e.g., `Certificate`, `Application`)

**Examples:**
- `cert-manager.io/v1/Certificate`
- `argoproj.io/v1alpha1/Application`
- `networking.istio.io/v1beta1/VirtualService`
- `monitoring.coreos.com/v1/ServiceMonitor`

**Note:** Core Kubernetes resources (empty group or "core" group) are not supported. Use their dedicated tables instead:
- ❌ `/v1/Pod` → Use `k8s_core_pods` table
- ❌ `/v1/Service` → Use `k8s_core_services` table
- ✅ `cert-manager.io/v1/Certificate` → Supported

### Namespace Filtering

You can optionally filter resources by namespace:

```yaml
custom_resources:
  # Fetch from specific namespaces
  - gvk: "cert-manager.io/v1/Certificate"
    namespaces: ["default", "production", "staging"]
  
  # Fetch from all namespaces (or cluster-scoped if applicable)
  - gvk: "cert-manager.io/v1/Issuer"
```

If `namespaces` is omitted or empty, the plugin fetches from all namespaces.

## Example Queries

### List all custom resources

```sql
SELECT context, api_version, kind, namespace, name, uid
FROM k8s_custom_resources
ORDER BY _cq_sync_time DESC;
```

### Find all cert-manager Certificates

```sql
SELECT 
  context,
  namespace,
  name,
  spec->>'secretName' AS secret_name,
  status->'conditions'->0->>'type' AS status
FROM k8s_custom_resources
WHERE api_version = 'cert-manager.io/v1'
  AND kind = 'Certificate';
```

### List resources by labels

```sql
SELECT name, namespace, labels->>'app' AS app
FROM k8s_custom_resources
WHERE labels->>'env' = 'production'
  AND api_version = 'argoproj.io/v1alpha1'
  AND kind = 'Application';
```

### Find resources synced in the last 7 days

```sql
SELECT api_version, kind, namespace, name, _cq_sync_time
FROM k8s_custom_resources
WHERE _cq_sync_time > NOW() - INTERVAL '7 days'
ORDER BY _cq_sync_time DESC;
```

### Check resource readiness

```sql
SELECT 
  name,
  namespace,
  status->'conditions' AS conditions
FROM k8s_custom_resources
WHERE api_version = 'cert-manager.io/v1'
  AND kind = 'Certificate'
  AND status->'conditions'->0->>'status' != 'True';
```

## Common Custom Resources

### Cert-Manager
```yaml
custom_resources:
  - gvk: "cert-manager.io/v1/Certificate"
  - gvk: "cert-manager.io/v1/Issuer"
  - gvk: "cert-manager.io/v1/ClusterIssuer"
  - gvk: "cert-manager.io/v1/CertificateRequest"
```

### ArgoCD
```yaml
custom_resources:
  - gvk: "argoproj.io/v1alpha1/Application"
  - gvk: "argoproj.io/v1alpha1/AppProject"
  - gvk: "argoproj.io/v1alpha1/ApplicationSet"
```

### Istio
```yaml
custom_resources:
  - gvk: "networking.istio.io/v1beta1/VirtualService"
  - gvk: "networking.istio.io/v1beta1/DestinationRule"
  - gvk: "networking.istio.io/v1beta1/Gateway"
  - gvk: "security.istio.io/v1beta1/PeerAuthentication"
```

### Prometheus Operator
```yaml
custom_resources:
  - gvk: "monitoring.coreos.com/v1/ServiceMonitor"
  - gvk: "monitoring.coreos.com/v1/PodMonitor"
  - gvk: "monitoring.coreos.com/v1/PrometheusRule"
  - gvk: "monitoring.coreos.com/v1/Prometheus"
```

## Troubleshooting

### Resource not found

**Error:** `failed to fetch resources for "example.com/v1/MyResource": the server could not find the requested resource`

**Solution:** 
- Verify the CRD is installed: `kubectl get crds`
- Check the GVK format is correct
- Ensure your kubeconfig has permissions to list the resource

### Invalid GVK format

**Error:** `invalid GVK format`

**Solution:** Ensure GVK follows the format `"group/version/kind"` with:
- Lowercase group and version
- Kind must start with uppercase letter
- Exactly 2 forward slashes

### Permission denied

**Error:** `failed to list resources: ... is forbidden`

**Solution:** 
- Verify your service account has RBAC permissions
- Grant list/get permissions for the custom resource:
  ```yaml
  apiVersion: rbac.authorization.k8s.io/v1
  kind: ClusterRole
  metadata:
    name: cloudquery-custom-resources
  rules:
  - apiGroups: ["cert-manager.io"]
    resources: ["certificates", "issuers"]
    verbs: ["get", "list"]
  ```

### Large spec/status causing issues

If spec or status fields are very large (>1MB), consider:
- Filtering fields in your SQL queries
- Using JSONB operators to extract specific fields
- Increasing database column size limits

## Notes

- The `spec` and `status` fields are JSON-encoded strings
- Use PostgreSQL's `::jsonb` cast to query nested fields
- Empty labels, annotations, spec, or status are represented as `{}`
- Cluster-scoped resources have an empty `namespace` field
- The plugin automatically handles pagination for large result sets
