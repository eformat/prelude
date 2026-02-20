# Prelude OpenShift Monitoring Dashboard

Enable the Prelude dashboard in the OpenShift web console under **Observe > Dashboards**.

This uses the same ConfigMap-based approach as the [NVIDIA GPU Monitoring Dashboard](https://docs.nvidia.com/datacenter/cloud-native/openshift/latest/enable-gpu-monitoring-dashboard.html). The dashboard JSON file (`prelude-dashboard.json`) is a Grafana 6.x format dashboard that queries the Prometheus metrics exposed by the prelude server on `:9090/metrics`.

## Prerequisites

- `oc` CLI logged in with `cluster-admin` privileges
- The prelude server is deployed with the ServiceMonitor enabled (`metrics.serviceMonitor.enabled: true` in Helm values)

## Step 1 -- Enable user workload monitoring

Skip this if user workload monitoring is already enabled on your cluster.

```bash
oc apply -f - <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: openshift-monitoring
data:
  config.yaml: |
    enableUserWorkload: true
EOF
```

## Step 2 -- Create the dashboard ConfigMap

```bash
oc create configmap prelude-dashboard \
  -n openshift-config-managed \
  --from-file=prelude-dashboard.json
```

## Step 3 -- Label it for the Administrator console

```bash
oc label configmap prelude-dashboard \
  -n openshift-config-managed \
  "console.openshift.io/dashboard=true"
```

## Step 4 (optional) -- Label it for the Developer console

```bash
oc label configmap prelude-dashboard \
  -n openshift-config-managed \
  "console.openshift.io/odc-dashboard=true"
```

## Step 5 -- Verify

```bash
oc -n openshift-config-managed get cm prelude-dashboard --show-labels
```

You should see both `console.openshift.io/dashboard=true` and optionally `console.openshift.io/odc-dashboard=true` in the labels.

## Step 6 -- View it

Navigate to **Observe > Dashboards** in the OpenShift web console. Select **Prelude Clusters** from the Dashboard dropdown.

## Dashboard panels

| Row | Panels | Description |
|-----|--------|-------------|
| **Cluster Summary** | 5 gauge panels | Deployments, Claims, Ready, Available, Claimed -- color-coded red/amber/green |
| **Cluster Utilization** | Utilization % gauge + stacked area chart | Claimed-vs-available ratio and availability over time |
| **Provisioning Pipeline** | All 5 metrics over time + utilization % trend | Full pipeline visibility with 70%/90% threshold lines |
| **Capacity Planning** | Pending authentication + unclaimed deployments | Shows clusters still being prepared and pool headroom |

## Prometheus metrics reference

| Metric | Description |
|--------|-------------|
| `prelude_cluster_deployments` | ClusterDeployments matching the pool |
| `prelude_cluster_claims` | ClusterClaims matching the pool |
| `prelude_clusters_ready` | ClusterClaims with `prelude-auth=done` |
| `prelude_clusters_available` | Ready clusters with no phone label |
| `prelude_clusters_claimed` | Ready clusters assigned to a user |

## Removing the dashboard

```bash
oc delete configmap prelude-dashboard -n openshift-config-managed
```
