# Prelude

Project prelude gives users access to OpenShift clusters when they provide their phone number and an admin password.

The server side is written in golang.

The client web app is written in Next.js and Tailwind CSS, styled to match the Red Hat brand design system.

## Server Side

All written in golang which must compile.

The server listens on `:8080` and exposes a `POST /api/claim` endpoint. It receives a phone number and admin password from the client.

The server requires a `--cluster-pool` flag to filter ClusterClaims by `spec.clusterPoolName`.

The server also accepts a `--cluster-lifetime` flag (default `2h`) to set the `spec.lifetime` on claimed ClusterClaims.

```bash
./server --cluster-pool prelude-lvtjv --cluster-lifetime 2h
```

Phone numbers are sanitized to valid Kubernetes label values (alphanumeric, `-`, `_`, `.`).

Looks up spoke cluster via the ClusterClaim in the hub OpenShift using the KUBECONFIG in the environment.

A command line equivalent would be:

```bash
oc get clusterclaim.hive.openshift.io -n cluster-pools
```

Only ClusterClaims matching the specified `--cluster-pool` are considered. The pool is identified by the `spec.clusterPoolName` field on the ClusterClaim.

If the label "prelude: phone-number" exists on the spoke clusterclaim - then return that cluster's web console URL.

We can get the spoke cluster web console url by doing the equivalent command line:

```bash
CLUSTER_NAME=roadshow-6kr2w
oc -n $CLUSTER_NAME get clusterdeployment $CLUSTER_NAME -o template='{{ index .status.webConsoleURL }}'
```

We can derive the spoke cluster ai console url by using the webConsoleURL as follows:

```bash
echo https://console-openshift-console.apps.roadshow-6kr2w.sandbox1763.opentlc.com |sed "s/console-openshift-console/data-science-gateway/"
```

We look up both the admin and user kubeconfig secrets from the spoke cluster. The admin kubeconfig secret name comes from `spec.clusterMetadata.adminKubeconfigSecretRef.name` on the ClusterDeployment. The user kubeconfig secret name is derived by replacing `-admin-kubeconfig` with `-user-kubeconfig`. For example:

- Admin kubeconfig secret: `roadshow-q8jzk-txg6b-0-dqfqp-admin-kubeconfig`
- User kubeconfig secret: `roadshow-q8jzk-txg6b-0-dqfqp-user-kubeconfig`

The admin kubeconfig is used internally to update the htpasswd secret on the spoke cluster. The user kubeconfig is returned to the client.

```bash
CLUSTER_NAME=roadshow-6kr2w
ADMIN_KUBECONFIG_SECRET_NAME=$(oc -n $CLUSTER_NAME get clusterdeployments $CLUSTER_NAME -ojsonpath='{.spec.clusterMetadata.adminKubeconfigSecretRef.name}')
USER_KUBECONFIG_SECRET_NAME=$(echo $ADMIN_KUBECONFIG_SECRET_NAME | sed 's/-admin-kubeconfig$/-user-kubeconfig/')
oc -n $CLUSTER_NAME get secret/$USER_KUBECONFIG_SECRET_NAME -o template='{{ .data }}'
```

If no ClusterClaim exists with the label "prelude: phone-number" grab the first ClusterClaim that does not have that label and label it by doing the equivalent command line:

```bash
CLUSTER_CLAIM_NAME=road1
PHONE_NUMBER=61-435-065-758
oc -n cluster-pools label clusterclaim.hive.openshift.io $CLUSTER_CLAIM_NAME prelude=$PHONE_NUMBER
```

When a cluster is claimed, the server sets `spec.lifetime` on the ClusterClaim to the ClusterClaim's current age plus the configured `--cluster-lifetime` value. Duration values support `d` (days), `h` (hours), and `m` (minutes) units (e.g. `2h`, `1d12h`, `30m`). The equivalent command line is:

```bash
oc -n cluster-pools patch clusterclaim.hive.openshift.io prelude1 --type merge -p '{"spec":{"lifetime":"2h"}}'
```

We can remove the label by doing the equivalent command line:

```bash
CLUSTER_CLAIM_NAME=road1
oc -n cluster-pools label clusterclaim.hive.openshift.io $CLUSTER_CLAIM_NAME prelude-
```

The admin password the user enters is written into an htpasswd secret. To generate htpasswd, the equivalent command line is:

```bash
htpasswd -bBc /tmp/htpasswd admin password
```

This must be stored in the spoke cluster using the returned spoke cluster KUBECONFIG. The htpass-secret is created if it does not exist, or updated if it does. The equivalent command line is:

```bash
oc get secret htpass-secret -n openshift-config -o template='{{ .data }}'
```

The server also returns an `expiresAt` field (RFC 3339 UTC timestamp) in the claim response, computed as the ClusterClaim's `creationTimestamp` plus `spec.lifetime`. For already-claimed clusters (phone number matches an existing label), the expiry is read from the existing `spec.lifetime`. For newly-claimed clusters, it uses the freshly computed lifetime.

If all the ClusterClaim's have a label "prelude: phone-number", and we cannot match the provided phone number, then display a nice message to the user - "All our clusters are in use at the moment, try again later".

## Cluster Claimer

A separate Go binary (`cluster-claimer/`) that automates initial cluster provisioning and claiming. A native Go implementation that uses a Kubernetes watch for efficient event-driven waiting.

The cluster-claimer requires one flag:

- `--cluster-pool` (or `CLUSTER_POOL` env var) — the ClusterPool name to watch (required)

```bash
./cluster-claimer --cluster-pool prelude-lvtjv
```

ClusterClaim names are derived automatically. The claimer compares provisioned ClusterDeployments against existing ClusterClaims for the pool, and creates claims for any gap using generated names (`prelude1`, `prelude2`, etc.), skipping names that already exist.

It performs the following steps in order:

1. **Watch for provisioned ClusterDeployments** — uses a Kubernetes watch on ClusterDeployments across all namespaces with the label `hive.openshift.io/clusterpool-name=<pool>`, waiting for the `Provisioned` condition to become `True`. Times out after 100 minutes.
2. **Determine claims needed** — counts provisioned ClusterDeployments and existing ClusterClaims for the pool. If all deployments already have claims, it idles (no-op).
3. **Create ClusterClaims** — creates ClusterClaim resources in the `cluster-pools` namespace named `prelude1`, `prelude2`, etc. with `spec.clusterPoolName` set and `system:masters` RBAC subject.

After completing (or if no claims are needed), the process idles to keep its container alive in the pod.

The cluster-claimer runs as a sidecar container in the same pod as the server and client, sharing the same kubeconfig volume. It runs asynchronously and independently of the other containers.

## Client Side

A Next.js 15, Tailwind CSS web app styled to match the Red Hat design system (Red Hat Display/Text fonts, Red Hat brand colors, dark hero section).

A user enters their phone number and an admin password and receives back a spoke cluster webConsoleURL, KUBECONFIG, and expiry time.

The phone number is validated client-side (7-15 digits). The password field has a reveal/hide toggle.

These are displayed in the web app with easy copy and download buttons displayed for the user. The Web Console URL card includes instructions to login with the "admin" user. A Cluster Lifetime card shows the expiry date/time in human-readable format and a live countdown timer.

The `/api/claim` call is made via a Next.js Server Action (not exposed to the browser). The client proxies `/api/config` to the Go server at `http://0.0.0.0:8080` via Next.js rewrites. The API URL is configurable via the `API_URL` environment variable.

## reCAPTCHA

Google reCAPTCHA v3 protects the `/api/claim` endpoint from bots. It is optional -- if the env vars are not set, verification is skipped.

Both env vars are set on the Go server container:

- `RECAPTCHA_SITE_KEY` — reCAPTCHA v3 site key (public). Served to the client at runtime via the `GET /api/config` endpoint.
- `RECAPTCHA_SECRET_KEY` — reCAPTCHA v3 secret key. Used server-side to verify tokens. When empty, reCAPTCHA verification is disabled.

## Build

```bash
make build-all              # Build server, cluster-claimer, and client
make build-server           # Build Go server
make build-cluster-claimer  # Build cluster-claimer
make build-client           # Build Next.js client
```

## Run (development)

```bash
make server-run             # Run Go server (port 8080)
make client-run             # Run Next.js dev server (port 3000)
make cluster-claimer-run    # Run cluster-claimer
make run-all                # Run server and client
```

## Container Images

```bash
make podman-build-all              # Build all container images
make podman-server-build           # Build server image (quay.io/eformat/prelude-server:latest)
make podman-cluster-claimer-build  # Build cluster-claimer image (quay.io/eformat/prelude-cluster-claimer:latest)
make podman-client-build           # Build client image (quay.io/eformat/prelude-client:latest)
```

Run with containers:

```bash
podman run --network host -p 8080:8080 -v ~/.kube/config:/root/.kube/config:Z quay.io/eformat/prelude-server:latest --cluster-pool prelude-lvtjv
podman run --network host -v ~/.kube/config:/root/.kube/config:Z quay.io/eformat/prelude-cluster-claimer:latest --cluster-pool prelude-lvtjv
podman run --network host -p 3000:3000 quay.io/eformat/prelude-client:latest
```
