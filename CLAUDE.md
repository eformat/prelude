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
./server --cluster-pool prelude-q8jzk --cluster-lifetime 2h
```

Phone numbers are sanitized to valid Kubernetes label values (alphanumeric, `-`, `_`, `.`).

Looks up spoke cluster via the ClusterClaim in the hub OpenShift using the KUBECONFIG in the environment.

A command line equivalent would be:

```bash
oc get clusterclaim.hive.openshift.io -n cluster-pools
```

Only ClusterClaims matching the specified `--cluster-pool` are considered. The pool is identified by the `spec.clusterPoolName` field on the ClusterClaim.

Only ClusterClaims with the label `prelude-auth=done` are eligible for assignment to users. This label is set by the cluster-authenticator after it has successfully prepared the cluster's kubeconfig credentials and spoke resources.

If the label "prelude: phone-number" exists on an eligible ClusterClaim - then return that cluster's web console URL.

We can get the spoke cluster web console url by doing the equivalent command line:

```bash
CLUSTER_NAME=prelude-q8jzk
oc -n $CLUSTER_NAME get clusterdeployment $CLUSTER_NAME -o template='{{ index .status.webConsoleURL }}'
```

We can derive the spoke cluster ai console url by using the webConsoleURL as follows:

```bash
echo https://console-openshift-console.apps.prelude-q8jzk.sandbox1763.opentlc.com |sed "s/console-openshift-console/data-science-gateway/"
```

We look up both the admin and user kubeconfig secrets from the spoke cluster. The admin kubeconfig secret name comes from `spec.clusterMetadata.adminKubeconfigSecretRef.name` on the ClusterDeployment. The user kubeconfig secret name is derived by replacing `-admin-kubeconfig` with `-user-kubeconfig`. For example:

- Admin kubeconfig secret: `prelude-q8jzk-txg6b-0-dqfqp-admin-kubeconfig`
- User kubeconfig secret: `prelude-q8jzk-txg6b-0-dqfqp-user-kubeconfig`

The admin kubeconfig is used internally to update the htpasswd secret on the spoke cluster. The user kubeconfig is returned to the client.

```bash
CLUSTER_NAME=prelude-q8jzk
ADMIN_KUBECONFIG_SECRET_NAME=$(oc -n $CLUSTER_NAME get clusterdeployments $CLUSTER_NAME -ojsonpath='{.spec.clusterMetadata.adminKubeconfigSecretRef.name}')
USER_KUBECONFIG_SECRET_NAME=$(echo $ADMIN_KUBECONFIG_SECRET_NAME | sed 's/-admin-kubeconfig$/-user-kubeconfig/')
oc -n $CLUSTER_NAME get secret/$USER_KUBECONFIG_SECRET_NAME -o template='{{ .data }}'
```

If no ClusterClaim exists with the label "prelude: phone-number" grab the first ClusterClaim that does not have that label and label it by doing the equivalent command line:

```bash
CLUSTER_CLAIM_NAME=road1
PHONE_NUMBER=61-435-999-768
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

The cluster-claimer accepts the following flags:

- `--cluster-pool` (or `CLUSTER_POOL` env var) — the ClusterPool name to watch (required)
- `--cluster-claim-limit` (or `CLUSTER_CLAIM_LIMIT` env var) — base number of ClusterClaims to create (default `4`)
- `--cluster-claim-max` (or `CLUSTER_CLAIM_MAX` env var) — maximum number of ClusterClaims when scaling up (default `10`)
- `--cluster-claim-increment` (or `CLUSTER_CLAIM_INCREMENT` env var) — number of claims to add each time the limit scales up (default `1`)

```bash
./cluster-claimer --cluster-pool prelude-q8jzk --cluster-claim-limit 4 --cluster-claim-max 10 --cluster-claim-increment 1
```

ClusterClaim names are derived automatically. The claimer compares provisioned ClusterDeployments against existing ClusterClaims for the pool, and creates claims for any gap using generated names (`prelude1`, `prelude2`, etc.), skipping names that already exist. The total number of claims is capped by the effective claim limit.

### Dynamic Claim Limit

The claim limit scales dynamically based on cluster availability. The effective limit starts at `--cluster-claim-limit` and increases when no clusters are available for users (authenticated but unclaimed). On each reconcile iteration, if there is 1 available cluster and the effective limit is below `--cluster-claim-max`, the limit increases by `--cluster-claim-increment` (capped at `--cluster-claim-max`). Scale-up has a 25-minute cooldown between increments, since clusters take approximately that long to become available after a ClusterClaim is created. A cluster is considered "available" when it has the `prelude-auth=done` label and no `prelude` phone label.

When clusters become available again, the effective limit scales back down to `--cluster-claim-limit` after a 10-minute hysteresis period. This prevents flapping — the limit only resets once clusters have been continuously available for 10 minutes. If availability drops to 0 during the hysteresis window, the timer resets and scale-up resumes immediately.

It performs the following steps:

1. **Wait for provisioned ClusterDeployments** — uses a Kubernetes watch on ClusterDeployments across all namespaces with the label `hive.openshift.io/clusterpool-name=<pool>`, waiting for the `Provisioned` condition to become `True`. Times out after 100 minutes.
2. **Reconciliation loop** — continuously watches ClusterDeployments and reconciles whenever a change is detected:
   - Counts provisioned ClusterDeployments and existing ClusterClaims for the pool.
   - If new claims are needed (up to `--cluster-claim-limit`), creates ClusterClaim resources named `prelude1`, `prelude2`, etc. with `spec.clusterPoolName` set and `system:masters` RBAC subject.
   - Watches for further ClusterDeployment changes (30s watch timeout) and re-reconciles when new deployments are added or become provisioned.

The cluster-claimer runs as a sidecar container in the same pod as the server and client, sharing the same kubeconfig volume. It runs asynchronously and independently of the other containers. It shuts down cleanly on SIGINT/SIGTERM.

## Cluster Authenticator

A separate Go binary (`cluster-authenticator/`) that automates the preparation of kubeconfig credentials once clusters are claimed. A native Go implementation that uses a Kubernetes watch for efficient event-driven processing.

The cluster-authenticator accepts the following flags:

- `--cluster-pool` (or `CLUSTER_POOL` env var) — the ClusterPool name to watch (required)

```bash
./cluster-authenticator --cluster-pool prelude-q8jzk
```

It watches ClusterClaims for the pool and processes each bound claim (one with `spec.namespace` set) that does not yet have the `prelude-auth=done` label. Each claim is processed exactly once. It performs the following steps:

1. **Get spoke admin kubeconfig** — retrieves the ClusterDeployment from `spec.namespace`, extracts `spec.clusterMetadata.adminKubeconfigSecretRef.name`, and builds a spoke REST client from the admin kubeconfig secret on the hub.

2. **Wait for stable cluster** — checks all ClusterOperators on the spoke cluster (`config.openshift.io/v1 clusteroperators`) for `Available=True`, `Progressing=False`, `Degraded=False`. All conditions must be stable for 120 seconds. Times out after 30 minutes. Equivalent to:

   ```bash
   oc adm wait-for-stable-cluster --minimum-stable-period=120s --timeout=30m
   ```

3. **Regenerate system:admin kubeconfig** — generates an RSA 4096 key pair, submits a CertificateSigningRequest (`kubernetes.io/kube-apiserver-client` signer) on the spoke cluster with `CN=system:admin`, approves it, extracts the signed certificate, retrieves the CA cert from the spoke API server TLS connection, and builds a kubeconfig YAML with embedded certs.

4. **Update admin kubeconfig secret on hub** — updates the admin kubeconfig secret (both `kubeconfig` and `raw-kubeconfig` keys) with the regenerated kubeconfig.

5. **Regenerate admin user kubeconfig** — same CSR flow as step 3 but with `CN=admin`.

6. **Create/update user kubeconfig secret on hub** — derives the user kubeconfig secret name (replacing `-admin-kubeconfig` with `-user-kubeconfig`), creates or updates the secret with the regenerated user kubeconfig.

7. **Create spoke resources** — using the new system:admin kubeconfig, creates on the spoke cluster (if they don't already exist):

   ```bash
   oc create configmap prelude -n openshift-config
   oc create secret generic htpass-secret \
       --from-literal=htpasswd="" \
       -n openshift-config
   ```

8. **Label claim as authenticated** — sets `prelude-auth=done` on the ClusterClaim, marking it as ready for users.

The cluster-authenticator runs as a sidecar container in the same pod as the server, client, and cluster-claimer, sharing the same kubeconfig volume. It runs asynchronously and independently. It shuts down cleanly on SIGINT/SIGTERM.

The server only considers ClusterClaims with the `prelude-auth=done` label when assigning clusters to users, ensuring users never receive a cluster that is still being prepared.


## Client Side

A Next.js 15, Tailwind CSS web app styled to match the Red Hat design system (Red Hat Display/Text fonts, Red Hat brand colors, dark hero section).

A user enters their phone number and an admin password and receives back a spoke cluster webConsoleURL, ai app console url, KUBECONFIG, and expiry time.

The phone number is validated client-side (7-15 digits). The password field has a reveal/hide toggle.

These are displayed in the web app with easy copy and download buttons displayed for the user. The Web Console URL card includes instructions to login with the "admin" user. A Cluster Lifetime card shows the expiry date/time in human-readable format and a live countdown timer.

The `/api/claim` call is made via a Next.js Server Action (not exposed to the browser). The client proxies `/api/config` to the Go server at `http://0.0.0.0:8080` via Next.js rewrites. The API URL is configurable via the `API_URL` environment variable.

Google Analytics is enabled via the Next.js `<Script>` component in the root layout, loaded with `afterInteractive` strategy on all pages.

### Firebase Phone Authentication

Phone numbers are verified via SMS using Firebase Authentication before a cluster is claimed. The flow is a two-step process:

1. **Send verification code** — the user enters their phone number (with country code, e.g. `+1234567890`) and admin password. The client creates a Firebase `RecaptchaVerifier` (invisible mode, attached to the submit button) and calls `signInWithPhoneNumber()` to send an SMS code.

2. **Verify code & claim** — the user enters the 6-digit SMS code. The client calls `confirmationResult.confirm(code)` to verify with Firebase. On success, it generates the browser fingerprint, executes the reCAPTCHA v3 token, and calls the `claimCluster` server action.

Firebase error codes are handled with user-friendly messages:
- `auth/invalid-phone-number` — prompts user to include country code
- `auth/too-many-requests` — rate limit message
- `auth/quota-exceeded` — SMS quota message
- `auth/invalid-verification-code` — invalid code message
- `auth/code-expired` — expired code, prompts resend

The Firebase config is in `client/app/firebase.ts` (excluded from git via `.gitignore`). This file must exist with a valid Firebase project config for phone auth to work. The config initializes `firebase/auth` and exports the `auth` instance.

### Admin page

The admin page at `/admin` provides a dashboard view of cluster status. It is accessed via the Next.js client and fetches data from the Go server's `GET /api/admin` endpoint through a Next.js Server Action (not exposed to the browser).

The Go server returns JSON with two arrays: `clusterClaims` (name, pool, phone, authenticated, namespace, age) and `clusterDeployments` (name, namespace, platform, region, version, provisionStatus, powerState, age). ClusterClaims are filtered by `--cluster-pool`. ClusterDeployments are queried across all namespaces by the label `hive.openshift.io/clusterpool-name=<pool>`.

The page displays:

- **Summary tiles** — Deployments, Claims, Ready (authenticated), Available (authenticated but unclaimed), Claimed (authenticated with phone label)
- **Cluster Claims table** — Name, Phone, Auth status (`done`/`pending`), Available (orange badge when `prelude-auth=done` and no `prelude` phone label), Namespace, Age
- **Cluster Deployments table** — Name, Platform, Region, Version, Provision Status (`Provisioned`/`Provisioning`), Power State, Age

The page auto-refreshes every 30 seconds with a manual refresh button.

### reCAPTCHA

Google reCAPTCHA v3 protects the app endpoints from bots. It is optional -- if the env vars are not set, verification is skipped.

Both env vars are set on the Go server container:

- `RECAPTCHA_SITE_KEY` — reCAPTCHA v3 site key (public). Served to the client at runtime via the `GET /api/config` endpoint.
- `RECAPTCHA_SECRET_KEY` — reCAPTCHA v3 secret key. Used server-side to verify tokens. When empty, reCAPTCHA verification is disabled.

### Admin Authentication

The admin page at `/admin` is protected by password authentication. It is optional -- if the env var is not set, the admin page is accessible without auth.

- `ADMIN_PASSWORD` — password required to access the admin dashboard. Set on the Go server container. When empty, admin authentication is disabled.

The authentication flow:

1. User navigates to `/admin` -- Next.js middleware checks for `prelude-admin-session` cookie
2. No cookie -- redirect to `/admin/login`
3. User enters password -- server action calls `POST /api/admin/login` on Go server
4. Go server validates password against `ADMIN_PASSWORD` env var -- returns a random session token
5. Server action sets `prelude-admin-session` cookie with the token value
6. User is redirected to `/admin`
7. The Go `GET /api/admin` endpoint validates the `Authorization: Bearer <token>` header for defense-in-depth
8. Tokens are stored in-memory on the Go server -- sessions are invalidated on server restart

### Browser Fingerprint Limiting

To prevent users from claiming multiple clusters with different phone numbers, a browser fingerprint is generated client-side and sent with the claim request. The fingerprint is a SHA-256 hash (first 16 hex characters) of stable browser properties: canvas rendering, screen dimensions, color depth, language, hardware concurrency, platform, and timezone.

The server stores the fingerprint as a `prelude-fp` label on the ClusterClaim. When a new claim is requested, the server checks if any existing claim has the same fingerprint but a different phone number, and rejects the request with a `device_already_claimed` error.

The client also stores `{ phone, fingerprint }` in `localStorage` key `prelude-claim` for fast client-side pre-validation (avoids a server round-trip).

**What it blocks:** Same browser/device with a different phone number (including incognito mode and cleared localStorage, since the server-side check is authoritative).

**What it allows:** Different browser or different device (acceptable trade-off).

## Helm Chart

A Helm chart in `chart/` deploys all four components as a single Pod with four containers (client, server, cluster-claimer, cluster-authenticator) sharing the same kubeconfig volume.

```bash
make helm-deploy                 # helm upgrade --install prelude ./chart
```

### Chart Structure

```
chart/
├── Chart.yaml                   # name: prelude, version: 0.1.0
├── values.yaml                  # Default configuration
└── templates/
    ├── deployment.yaml          # Single Pod with 4 containers
    ├── service.yaml
    ├── route.yaml               # OpenShift Route
    ├── serviceaccount.yaml
    ├── clusterrole.yaml
    ├── clusterrolebinding.yaml
    └── _helpers.tpl
```

### Configuration (values.yaml)

```yaml
server:
  image:
    repository: quay.io/eformat/prelude-server
    tag: latest
  clusterPool: ""                # Required — ClusterPool name
  clusterLifetime: "2h"
  kubeconfigSecret: ""           # Kubernetes Secret name mounted as KUBECONFIG
  recaptchaSiteKey: ""
  recaptchaSecretKey: ""
  adminPassword: ""

clusterClaimer:
  image:
    repository: quay.io/eformat/prelude-cluster-claimer
    tag: latest
  clusterClaimLimit: "4"
  clusterClaimMax: "10"
  clusterClaimIncrement: "1"

clusterAuthenticator:
  image:
    repository: quay.io/eformat/prelude-cluster-authenticator
    tag: latest

client:
  image:
    repository: quay.io/eformat/prelude-client
    tag: latest

route:
  host: ""                       # OpenShift Route hostname
```

### Deployment Architecture

The Deployment creates a single Pod with four containers:

- **client** — port 3000, serves the Next.js app
- **server** — Go API server (internal to pod, no exposed port)
- **cluster-claimer** — watches for provisioned ClusterDeployments and creates ClusterClaims
- **cluster-authenticator** — processes bound ClusterClaims and prepares kubeconfig credentials

All containers share the same `CLUSTER_POOL` env var. If `kubeconfigSecret` is set, all containers mount the Secret at `/etc/prelude/kubeconfig/kubeconfig` and set the `KUBECONFIG` env var. RBAC is configured via a ServiceAccount with a ClusterRole and ClusterRoleBinding. The Service routes traffic to the client container on port 3000, and an OpenShift Route exposes it externally.

## Build

```bash
make build-all                    # Build server, cluster-claimer, cluster-authenticator, and client
make build-server                 # Build Go server
make build-cluster-claimer        # Build cluster-claimer
make build-cluster-authenticator  # Build cluster-authenticator
make build-client                 # Build Next.js client
```

## Run (development)

```bash
make server-run                 # Run Go server (port 8080)
make client-run                 # Run Next.js dev server (port 3000)
make cluster-claimer-run        # Run cluster-claimer
make cluster-authenticator-run  # Run cluster-authenticator
make run-all                    # Run server and client
```

## Container Images

```bash
make podman-build-all                    # Build all container images
make podman-server-build                 # Build server image (quay.io/eformat/prelude-server:latest)
make podman-cluster-claimer-build        # Build cluster-claimer image (quay.io/eformat/prelude-cluster-claimer:latest)
make podman-cluster-authenticator-build  # Build cluster-authenticator image (quay.io/eformat/prelude-cluster-authenticator:latest)
make podman-client-build                 # Build client image (quay.io/eformat/prelude-client:latest)
```

Run with containers:

```bash
podman run --network host -p 8080:8080 -v ~/.kube/config:/root/.kube/config:Z quay.io/eformat/prelude-server:latest --cluster-pool prelude-q8jzk
podman run --network host -v ~/.kube/config:/root/.kube/config:Z quay.io/eformat/prelude-cluster-claimer:latest --cluster-pool prelude-q8jzk
podman run --network host -v ~/.kube/config:/root/.kube/config:Z quay.io/eformat/prelude-cluster-authenticator:latest --cluster-pool prelude-q8jzk
podman run --network host -p 3000:3000 quay.io/eformat/prelude-client:latest
```
