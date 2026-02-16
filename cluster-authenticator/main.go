package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	clusterClaimGVR = schema.GroupVersionResource{
		Group:    "hive.openshift.io",
		Version:  "v1",
		Resource: "clusterclaims",
	}
	clusterDeploymentGVR = schema.GroupVersionResource{
		Group:    "hive.openshift.io",
		Version:  "v1",
		Resource: "clusterdeployments",
	}
	clusterOperatorGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusteroperators",
	}
	clusterPoolNamespace = "cluster-pools"
)

func main() {
	clusterPool := flag.String("cluster-pool", os.Getenv("CLUSTER_POOL"), "ClusterPool name to filter by (required)")
	flag.Parse()

	if *clusterPool == "" {
		log.Fatalf("--cluster-pool flag or CLUSTER_POOL environment variable is required")
	}

	log.Printf("Cluster pool: %s", *clusterPool)

	config, err := buildConfig()
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	hubDynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating dynamic client: %v", err)
	}

	hubClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating kubernetes client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("Received shutdown signal")
		cancel()
	}()

	reconcile(ctx, hubDynClient, hubClientset, *clusterPool)
	log.Printf("Cluster authenticator shutting down")
}

// reconcile continuously watches ClusterClaims and authenticates bound claims
// that haven't been processed yet.
func reconcile(ctx context.Context, hubDynClient dynamic.Interface, hubClientset kubernetes.Interface, pool string) {
	for {
		if ctx.Err() != nil {
			return
		}

		processUnauthenticatedClaims(ctx, hubDynClient, hubClientset, pool)

		// Watch for ClusterClaim changes, then re-reconcile
		var timeoutSecs int64 = 30
		list, err := hubDynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Error listing ClusterClaims: %v", err)
			sleepOrDone(ctx, 10*time.Second)
			continue
		}

		watcher, err := hubDynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).Watch(ctx, metav1.ListOptions{
			TimeoutSeconds:  &timeoutSecs,
			ResourceVersion: list.GetResourceVersion(),
		})
		if err != nil {
			log.Printf("Error watching ClusterClaims: %v", err)
			sleepOrDone(ctx, 10*time.Second)
			continue
		}

		for event := range watcher.ResultChan() {
			if event.Type == watch.Added || event.Type == watch.Modified {
				break
			}
		}
		watcher.Stop()
	}
}

// processUnauthenticatedClaims finds bound ClusterClaims without the
// prelude-auth=done label and processes them.
func processUnauthenticatedClaims(ctx context.Context, hubDynClient dynamic.Interface, hubClientset kubernetes.Interface, pool string) {
	claims, err := hubDynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Error listing ClusterClaims: %v", err)
		return
	}

	for _, claim := range claims.Items {
		if ctx.Err() != nil {
			return
		}

		if !claimMatchesPool(claim.Object, pool) {
			continue
		}

		// Check if already authenticated
		labels := claim.GetLabels()
		if labels != nil && labels["prelude-auth"] == "done" {
			continue
		}

		// Check if bound (has spec.namespace)
		clusterName := getSpecNamespace(claim.Object)
		if clusterName == "" {
			continue
		}

		claimName := claim.GetName()
		log.Printf("Processing unauthenticated claim %s (cluster: %s)", claimName, clusterName)

		if err := authenticateCluster(ctx, hubDynClient, hubClientset, claimName, clusterName); err != nil {
			log.Printf("Error authenticating cluster %s (claim %s): %v", clusterName, claimName, err)
			continue
		}

		// Label claim as authenticated
		if err := labelClaimAuthenticated(ctx, hubDynClient, claimName); err != nil {
			log.Printf("Error labeling claim %s as authenticated: %v", claimName, err)
			continue
		}

		log.Printf("Successfully authenticated cluster %s (claim %s)", clusterName, claimName)
	}
}

// authenticateCluster performs the full authentication flow for a spoke cluster.
func authenticateCluster(ctx context.Context, hubDynClient dynamic.Interface, hubClientset kubernetes.Interface, claimName, clusterName string) error {
	// Step 1: Get spoke admin kubeconfig from hub
	log.Printf("[%s] Getting ClusterDeployment", clusterName)
	cd, err := hubDynClient.Resource(clusterDeploymentGVR).Namespace(clusterName).Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting ClusterDeployment: %w", err)
	}

	adminSecretName := getAdminKubeconfigSecretName(cd.Object)
	if adminSecretName == "" {
		return fmt.Errorf("could not find adminKubeconfigSecretRef in ClusterDeployment %s", clusterName)
	}
	log.Printf("[%s] Admin kubeconfig secret: %s", clusterName, adminSecretName)

	adminSecret, err := hubClientset.CoreV1().Secrets(clusterName).Get(ctx, adminSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting admin kubeconfig secret: %w", err)
	}

	spokeKubeconfigData := extractKubeconfig(adminSecret)
	if spokeKubeconfigData == "" {
		return fmt.Errorf("admin kubeconfig secret %s has no kubeconfig data", adminSecretName)
	}

	spokeConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(spokeKubeconfigData))
	if err != nil {
		return fmt.Errorf("building spoke REST config: %w", err)
	}

	spokeDynClient, err := dynamic.NewForConfig(spokeConfig)
	if err != nil {
		return fmt.Errorf("creating spoke dynamic client: %w", err)
	}

	spokeClientset, err := kubernetes.NewForConfig(spokeConfig)
	if err != nil {
		return fmt.Errorf("creating spoke typed client: %w", err)
	}

	// Step 2: Wait for stable cluster
	log.Printf("[%s] Waiting for cluster to stabilize", clusterName)
	if err := waitForStableCluster(ctx, spokeDynClient, clusterName); err != nil {
		return fmt.Errorf("waiting for stable cluster: %w", err)
	}
	log.Printf("[%s] Cluster is stable", clusterName)

	// Step 3: Regenerate system:admin kubeconfig via CSR
	log.Printf("[%s] Regenerating system:admin kubeconfig", clusterName)
	adminKubeconfig, err := regenerateKubeconfig(ctx, spokeClientset, spokeConfig, "system:admin", "auth2kube-systemadmin-access")
	if err != nil {
		return fmt.Errorf("regenerating system:admin kubeconfig: %w", err)
	}

	// Step 4: Update admin kubeconfig secret on hub
	log.Printf("[%s] Updating admin kubeconfig secret on hub", clusterName)
	adminSecret.Data["kubeconfig"] = []byte(adminKubeconfig)
	adminSecret.Data["raw-kubeconfig"] = []byte(adminKubeconfig)
	if _, err := hubClientset.CoreV1().Secrets(clusterName).Update(ctx, adminSecret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating admin kubeconfig secret: %w", err)
	}

	// Step 5: Regenerate admin user kubeconfig via CSR
	log.Printf("[%s] Regenerating admin user kubeconfig", clusterName)
	userKubeconfig, err := regenerateKubeconfig(ctx, spokeClientset, spokeConfig, "admin", "auth2kube-admin-access")
	if err != nil {
		return fmt.Errorf("regenerating admin user kubeconfig: %w", err)
	}

	// Step 6: Create/update user kubeconfig secret on hub
	userSecretName := strings.Replace(adminSecretName, "-admin-kubeconfig", "-user-kubeconfig", 1)
	log.Printf("[%s] Creating/updating user kubeconfig secret %s on hub", clusterName, userSecretName)
	if err := createOrUpdateSecret(ctx, hubClientset, clusterName, userSecretName, userKubeconfig); err != nil {
		return fmt.Errorf("creating/updating user kubeconfig secret: %w", err)
	}

	// Step 7: Create spoke resources using the NEW system:admin kubeconfig
	log.Printf("[%s] Creating spoke resources", clusterName)
	newSpokeConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(adminKubeconfig))
	if err != nil {
		return fmt.Errorf("building new spoke REST config: %w", err)
	}
	newSpokeClientset, err := kubernetes.NewForConfig(newSpokeConfig)
	if err != nil {
		return fmt.Errorf("creating new spoke client: %w", err)
	}
	if err := createSpokeResources(ctx, newSpokeClientset, clusterName); err != nil {
		return fmt.Errorf("creating spoke resources: %w", err)
	}

	return nil
}

// waitForStableCluster waits for all ClusterOperators to be stable for a
// minimum period, equivalent to: oc adm wait-for-stable-cluster --minimum-stable-period=120s --timeout=30m
func waitForStableCluster(ctx context.Context, spokeDynClient dynamic.Interface, clusterName string) error {
	timeout := 30 * time.Minute
	stablePeriod := 120 * time.Second
	deadline := time.Now().Add(timeout)

	var stableSince *time.Time

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for cluster %s to stabilize after %v", clusterName, timeout)
		}

		stable, err := areClusterOperatorsStable(ctx, spokeDynClient)
		if err != nil {
			log.Printf("[%s] Error checking ClusterOperators: %v", clusterName, err)
			stableSince = nil
			sleepOrDone(ctx, 10*time.Second)
			continue
		}

		if stable {
			now := time.Now()
			if stableSince == nil {
				stableSince = &now
				log.Printf("[%s] All ClusterOperators stable, waiting for %v stable period", clusterName, stablePeriod)
			}
			if time.Since(*stableSince) >= stablePeriod {
				return nil
			}
		} else {
			if stableSince != nil {
				log.Printf("[%s] ClusterOperators became unstable, resetting stable period", clusterName)
			}
			stableSince = nil
		}

		sleepOrDone(ctx, 10*time.Second)
	}
}

// areClusterOperatorsStable checks if all ClusterOperators have
// Available=True, Progressing=False, Degraded=False.
func areClusterOperatorsStable(ctx context.Context, spokeDynClient dynamic.Interface) (bool, error) {
	list, err := spokeDynClient.Resource(clusterOperatorGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing ClusterOperators: %w", err)
	}

	if len(list.Items) == 0 {
		return false, nil
	}

	for _, co := range list.Items {
		status, ok := co.Object["status"].(map[string]interface{})
		if !ok {
			return false, nil
		}
		conditions, ok := status["conditions"].([]interface{})
		if !ok {
			return false, nil
		}

		condMap := make(map[string]string)
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)
			condMap[condType] = condStatus
		}

		if condMap["Available"] != "True" || condMap["Progressing"] != "False" || condMap["Degraded"] != "False" {
			return false, nil
		}
	}

	return true, nil
}

// regenerateKubeconfig generates a new kubeconfig for the given CN via the
// Kubernetes CSR flow on the spoke cluster.
func regenerateKubeconfig(ctx context.Context, spokeClientset kubernetes.Interface, spokeConfig *rest.Config, cn, csrName string) (string, error) {
	// Generate RSA 4096 key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", fmt.Errorf("generating RSA key: %w", err)
	}

	// Create X.509 CSR
	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: cn,
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, privateKey)
	if err != nil {
		return "", fmt.Errorf("creating CSR: %w", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Delete old CSR if it exists
	_ = spokeClientset.CertificatesV1().CertificateSigningRequests().Delete(ctx, csrName, metav1.DeleteOptions{})

	// Submit CSR to spoke cluster
	var expirationSeconds int32 = 31536000 // one year
	k8sCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrName,
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:           csrPEM,
			SignerName:        "kubernetes.io/kube-apiserver-client",
			ExpirationSeconds: &expirationSeconds,
			Usages:            []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth},
			Groups:            []string{"system:authenticated"},
		},
	}

	createdCSR, err := spokeClientset.CertificatesV1().CertificateSigningRequests().Create(ctx, k8sCSR, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("creating CSR resource: %w", err)
	}
	log.Printf("CSR %s created for CN=%s", csrName, cn)

	// Approve CSR
	createdCSR.Status.Conditions = append(createdCSR.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
		Type:               certificatesv1.CertificateApproved,
		Status:             corev1.ConditionTrue,
		Reason:             "PreludeAuthenticator",
		Message:            "Approved by cluster-authenticator",
		LastUpdateTime:     metav1.Now(),
	})
	_, err = spokeClientset.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, csrName, createdCSR, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("approving CSR: %w", err)
	}
	log.Printf("CSR %s approved", csrName)

	// Wait for signed certificate
	var certPEM []byte
	for i := 0; i < 30; i++ {
		csr, err := spokeClientset.CertificatesV1().CertificateSigningRequests().Get(ctx, csrName, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("getting CSR status: %w", err)
		}
		if len(csr.Status.Certificate) > 0 {
			certPEM = csr.Status.Certificate
			break
		}
		sleepOrDone(ctx, 2*time.Second)
	}
	if certPEM == nil {
		return "", fmt.Errorf("timed out waiting for CSR %s certificate", csrName)
	}
	log.Printf("CSR %s certificate issued", csrName)

	// Extract CA cert from TLS connection to spoke API server
	caCertPEM, err := extractCACert(spokeConfig.Host)
	if err != nil {
		return "", fmt.Errorf("extracting CA cert: %w", err)
	}

	// Encode private key to PEM
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Build kubeconfig YAML
	kubeconfig := buildKubeconfigYAML(spokeConfig.Host, cn, caCertPEM, certPEM, keyPEM)
	return kubeconfig, nil
}

// extractCACert extracts the CA certificate from a TLS connection to the API server.
func extractCACert(host string) ([]byte, error) {
	// Strip scheme if present
	addr := host
	if strings.HasPrefix(addr, "https://") {
		addr = strings.TrimPrefix(addr, "https://")
	} else if strings.HasPrefix(addr, "http://") {
		addr = strings.TrimPrefix(addr, "http://")
	}

	// Ensure port is present
	if !strings.Contains(addr, ":") {
		addr = addr + ":6443"
	}

	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, fmt.Errorf("TLS dial to %s: %w", addr, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates from %s", addr)
	}

	// Use the last cert in the chain (root CA) or any CA cert
	var caCert *x509.Certificate
	for i := len(certs) - 1; i >= 0; i-- {
		if certs[i].IsCA {
			caCert = certs[i]
			break
		}
	}
	if caCert == nil {
		// Fall back to last cert in chain
		caCert = certs[len(certs)-1]
	}

	caPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCert.Raw,
	})
	return caPEM, nil
}

// buildKubeconfigYAML creates a kubeconfig YAML string with embedded certificates.
func buildKubeconfigYAML(server, user string, caCert, clientCert, clientKey []byte) string {
	caB64 := base64.StdEncoding.EncodeToString(caCert)
	certB64 := base64.StdEncoding.EncodeToString(clientCert)
	keyB64 := base64.StdEncoding.EncodeToString(clientKey)

	// Derive cluster name from server URL
	clusterName := "cluster"
	if strings.Contains(server, "api.") {
		// Extract something like "api.roadshow-xxx.sandbox.opentlc.com" -> use full hostname
		host := strings.TrimPrefix(server, "https://")
		host = strings.TrimPrefix(host, "http://")
		host = strings.Split(host, ":")[0]
		clusterName = host
	}

	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: %s
    server: %s
  name: %s
contexts:
- context:
    cluster: %s
    namespace: default
    user: %s
  name: %s
current-context: %s
preferences: {}
users:
- name: %s
  user:
    client-certificate-data: %s
    client-key-data: %s
`, caB64, server, clusterName, clusterName, user, user, user, user, certB64, keyB64)
}

// createOrUpdateSecret creates or updates a secret with kubeconfig data.
func createOrUpdateSecret(ctx context.Context, hubClientset kubernetes.Interface, namespace, name, kubeconfig string) error {
	secret, err := hubClientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"kubeconfig":     []byte(kubeconfig),
				"raw-kubeconfig": []byte(kubeconfig),
			},
		}
		_, err = hubClientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		return err
	} else if err != nil {
		return err
	}

	secret.Data["kubeconfig"] = []byte(kubeconfig)
	secret.Data["raw-kubeconfig"] = []byte(kubeconfig)
	_, err = hubClientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	return err
}

// createSpokeResources creates the prerequisite resources on the spoke cluster
// for ACM policy deployment.
func createSpokeResources(ctx context.Context, spokeClientset kubernetes.Interface, clusterName string) error {
	// Create configmap prelude in openshift-config (if not exists)
	_, err := spokeClientset.CoreV1().ConfigMaps("openshift-config").Get(ctx, "prelude", metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prelude",
				Namespace: "openshift-config",
			},
		}
		if _, err := spokeClientset.CoreV1().ConfigMaps("openshift-config").Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating prelude configmap: %w", err)
		}
		log.Printf("[%s] Created prelude configmap in openshift-config", clusterName)
	} else if err != nil {
		return fmt.Errorf("checking prelude configmap: %w", err)
	} else {
		log.Printf("[%s] Prelude configmap already exists in openshift-config", clusterName)
	}

	// Create secret htpass-secret in openshift-config (if not exists)
	_, err = spokeClientset.CoreV1().Secrets("openshift-config").Get(ctx, "htpass-secret", metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "htpass-secret",
				Namespace: "openshift-config",
			},
			Data: map[string][]byte{
				"htpasswd": []byte(""),
			},
		}
		if _, err := spokeClientset.CoreV1().Secrets("openshift-config").Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating htpass-secret: %w", err)
		}
		log.Printf("[%s] Created htpass-secret in openshift-config", clusterName)
	} else if err != nil {
		return fmt.Errorf("checking htpass-secret: %w", err)
	} else {
		log.Printf("[%s] htpass-secret already exists in openshift-config", clusterName)
	}

	return nil
}

// labelClaimAuthenticated sets the prelude-authenticated=true label on a ClusterClaim.
func labelClaimAuthenticated(ctx context.Context, hubDynClient dynamic.Interface, claimName string) error {
	claim, err := hubDynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting claim: %w", err)
	}
	labels := claim.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["prelude-auth"] = "done"
	claim.SetLabels(labels)
	_, err = hubDynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).Update(ctx, claim, metav1.UpdateOptions{})
	return err
}

// claimMatchesPool checks if a ClusterClaim belongs to the specified ClusterPool.
func claimMatchesPool(obj map[string]interface{}, poolName string) bool {
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return false
	}
	name, ok := spec["clusterPoolName"].(string)
	if !ok {
		return false
	}
	return name == poolName
}

// getSpecNamespace returns spec.namespace from a ClusterClaim, or empty if not set.
func getSpecNamespace(obj map[string]interface{}) string {
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return ""
	}
	ns, ok := spec["namespace"].(string)
	if !ok {
		return ""
	}
	return ns
}

// getAdminKubeconfigSecretName extracts spec.clusterMetadata.adminKubeconfigSecretRef.name
// from a ClusterDeployment object.
func getAdminKubeconfigSecretName(obj map[string]interface{}) string {
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return ""
	}
	meta, ok := spec["clusterMetadata"].(map[string]interface{})
	if !ok {
		return ""
	}
	ref, ok := meta["adminKubeconfigSecretRef"].(map[string]interface{})
	if !ok {
		return ""
	}
	name, ok := ref["name"].(string)
	if !ok {
		return ""
	}
	return name
}

// extractKubeconfig reads kubeconfig data from a Secret, handling common key names
// and base64-encoded values.
func extractKubeconfig(secret *corev1.Secret) string {
	var data string
	if raw, ok := secret.Data["kubeconfig"]; ok {
		data = string(raw)
	} else if raw, ok := secret.Data["raw-kubeconfig"]; ok {
		data = string(raw)
	} else {
		for _, v := range secret.Data {
			data = string(v)
			break
		}
	}
	if decoded, err := base64.StdEncoding.DecodeString(data); err == nil && len(decoded) > 0 && strings.Contains(string(decoded), "apiVersion") {
		data = string(decoded)
	}
	return data
}

// sleepOrDone sleeps for the given duration or returns early if the context is cancelled.
func sleepOrDone(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// buildConfig returns a Kubernetes REST config. It uses the KUBECONFIG env var
// or ~/.kube/config if available, otherwise falls back to in-cluster config.
func buildConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			candidate := filepath.Join(home, ".kube", "config")
			if _, err := os.Stat(candidate); err == nil {
				kubeconfig = candidate
			}
		}
	}
	if kubeconfig != "" {
		log.Printf("Using kubeconfig: %s", kubeconfig)
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	log.Printf("Using in-cluster config")
	return rest.InClusterConfig()
}
