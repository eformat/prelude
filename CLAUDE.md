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
./server --cluster-pool roadshow-lvtjv --cluster-lifetime 2h
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

When a cluster is claimed, the server also patches `spec.lifetime` on the ClusterClaim with the configured `--cluster-lifetime` value. The equivalent command line is:

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

If all the ClusterClaim's have a label "prelude: phone-number", and we cannot match the provided phone number, then display a nice message to the user - "All our clusters are in use at the moment, try again later".

## Client Side

A Next.js 15, Tailwind CSS web app styled to match the Red Hat design system (Red Hat Display/Text fonts, Red Hat brand colors, dark hero section).

A user enters their phone number and an admin password and receives back a spoke cluster webConsoleURL and KUBECONFIG.

The phone number is validated client-side (7-15 digits). The password field has a reveal/hide toggle.

These are displayed in the web app with easy copy and download buttons displayed for the user. The Web Console URL card includes instructions to login with the "admin" user.

The client proxies `/api/*` requests to the Go server at `http://0.0.0.0:8080` in both dev mode (via Next.js rewrites) and standalone container mode. The API URL is configurable via the `API_URL` environment variable.

## reCAPTCHA

Google reCAPTCHA v3 protects the `/api/claim` endpoint from bots. It is optional -- if the env vars are not set, verification is skipped.

Both env vars are set on the Go server container:

- `RECAPTCHA_SITE_KEY` — reCAPTCHA v3 site key (public). Served to the client at runtime via the `GET /api/config` endpoint.
- `RECAPTCHA_SECRET_KEY` — reCAPTCHA v3 secret key. Used server-side to verify tokens. When empty, reCAPTCHA verification is disabled.

## Build

```bash
make build-all        # Build both server and client
make build-server     # Build Go server
make build-client     # Build Next.js client
```

## Run (development)

```bash
make server-run       # Run Go server (port 8080)
make client-run       # Run Next.js dev server (port 3000)
make run-all          # Run both
```

## Container Images

```bash
make podman-build-all       # Build both container images
make podman-server-build    # Build server image (quay.io/eformat/prelude-server:latest)
make podman-client-build    # Build client image (quay.io/eformat/prelude-client:latest)
```

Run with containers:

```bash
podman run --network host -p 8080:8080 -v ~/.kube/config:/root/.kube/config:Z quay.io/eformat/prelude-server:latest --cluster-pool roadshow-lvtjv
podman run --network host -p 3000:3000 quay.io/eformat/prelude-client:latest
```
