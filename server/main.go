package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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
	clusterPoolNamespace  = "cluster-pools"
	recaptchaVerifyURL    = "https://www.google.com/recaptcha/api/siteverify"
	recaptchaMinScore     = 0.5
)

var recaptchaSecretKey string
var recaptchaSiteKey string

type claimRequest struct {
	Phone          string `json:"phone"`
	Password       string `json:"password"`
	RecaptchaToken string `json:"recaptchaToken"`
}

// sanitizePhone converts a phone number into a valid Kubernetes label value.
// Valid labels: alphanumeric, '-', '_', '.', must start/end with alphanumeric.
func sanitizePhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else if r == ' ' || r == '+' || r == '(' || r == ')' {
			b.WriteRune('-')
		}
	}
	// Trim leading/trailing non-alphanumeric characters
	result := strings.Trim(b.String(), "-_.")
	return result
}

// parseDuration parses a duration string supporting d (days), h (hours), and m (minutes).
// Examples: "2h", "30m", "1d", "1d12h", "2h30m".
func parseDuration(s string) (time.Duration, error) {
	var total time.Duration
	current := ""
	for _, c := range s {
		if c >= '0' && c <= '9' {
			current += string(c)
		} else {
			if current == "" {
				return 0, fmt.Errorf("invalid duration: %s", s)
			}
			n, err := strconv.Atoi(current)
			if err != nil {
				return 0, fmt.Errorf("invalid duration: %s", s)
			}
			switch c {
			case 'd':
				total += time.Duration(n) * 24 * time.Hour
			case 'h':
				total += time.Duration(n) * time.Hour
			case 'm':
				total += time.Duration(n) * time.Minute
			default:
				return 0, fmt.Errorf("invalid duration unit %q in: %s", string(c), s)
			}
			current = ""
		}
	}
	if current != "" {
		return 0, fmt.Errorf("invalid duration (trailing number without unit): %s", s)
	}
	return total, nil
}

// formatDuration formats a duration using d, h, m units.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	var parts []string
	days := int(d.Hours()) / 24
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
		d -= time.Duration(days) * 24 * time.Hour
	}
	hours := int(d.Hours())
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
		d -= time.Duration(hours) * time.Hour
	}
	minutes := int(d.Minutes())
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 {
		return "1m"
	}
	return strings.Join(parts, "")
}

type claimResponse struct {
	WebConsoleURL string `json:"webConsoleURL"`
	AIConsoleURL  string `json:"aiConsoleURL"`
	Kubeconfig    string `json:"kubeconfig"`
}

type recaptchaResponse struct {
	Success bool    `json:"success"`
	Score   float64 `json:"score"`
}

func verifyRecaptcha(token string) error {
	resp, err := http.PostForm(recaptchaVerifyURL, url.Values{
		"secret":   {recaptchaSecretKey},
		"response": {token},
	})
	if err != nil {
		return fmt.Errorf("recaptcha request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading recaptcha response: %w", err)
	}

	var result recaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing recaptcha response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("recaptcha verification failed")
	}

	if result.Score < recaptchaMinScore {
		return fmt.Errorf("recaptcha score %.2f below threshold %.2f", result.Score, recaptchaMinScore)
	}

	return nil
}

func main() {
	clusterPool := flag.String("cluster-pool", os.Getenv("CLUSTER_POOL"), "ClusterPool name to filter ClusterClaims by (required)")
	clusterLifetime := flag.String("cluster-lifetime", os.Getenv("CLUSTER_LIFETIME"), "Lifetime to set on claimed ClusterClaims (e.g. 2h)")
	flag.Parse()

	if *clusterPool == "" {
		log.Fatalf("--cluster-pool flag or CLUSTER_POOL environment variable is required")
	}
	if *clusterLifetime == "" {
		*clusterLifetime = "2h"
	}
	recaptchaSecretKey = os.Getenv("RECAPTCHA_SECRET_KEY")
	recaptchaSiteKey = os.Getenv("RECAPTCHA_SITE_KEY")
	if recaptchaSecretKey != "" {
		log.Printf("reCAPTCHA verification enabled")
	} else {
		log.Printf("reCAPTCHA verification disabled (RECAPTCHA_SECRET_KEY not set)")
	}

	log.Printf("Filtering ClusterClaims by clusterPoolName: %s", *clusterPool)
	log.Printf("Cluster lifetime: %s", *clusterLifetime)

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating dynamic client: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating kubernetes client: %v", err)
	}

	pool := *clusterPool
	lifetime := *clusterLifetime
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/claim", func(w http.ResponseWriter, r *http.Request) {
		handleClaim(w, r, dynClient, clientset, pool, lifetime)
	})

	staticDir := filepath.Join("..", "client", "out")
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))

	addr := ":8080"
	log.Printf("Server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"recaptchaSiteKey": recaptchaSiteKey,
	})
}

func handleClaim(w http.ResponseWriter, r *http.Request, dynClient dynamic.Interface, clientset kubernetes.Interface, clusterPool string, clusterLifetime string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req claimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify reCAPTCHA token if secret key is configured
	if recaptchaSecretKey != "" {
		if req.RecaptchaToken == "" {
			http.Error(w, "reCAPTCHA token is required", http.StatusForbidden)
			return
		}
		if err := verifyRecaptcha(req.RecaptchaToken); err != nil {
			log.Printf("reCAPTCHA verification failed: %v", err)
			http.Error(w, "reCAPTCHA verification failed", http.StatusForbidden)
			return
		}
	}

	phone := sanitizePhone(strings.TrimSpace(req.Phone))
	if phone == "" {
		http.Error(w, "Phone number is required", http.StatusBadRequest)
		return
	}

	password := strings.TrimSpace(req.Password)
	if password == "" {
		http.Error(w, "Admin password is required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// List all ClusterClaims in cluster-pools namespace
	claims, err := dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Error listing cluster claims: %v", err)
		http.Error(w, "Failed to list cluster claims", http.StatusInternalServerError)
		return
	}

	var claimName string
	var clusterName string
	found := false

	// Check if any ClusterClaim already has this phone number
	for _, claim := range claims.Items {
		if !claimMatchesPool(claim.Object, clusterPool) {
			continue
		}
		labels := claim.GetLabels()
		if labels != nil && labels["prelude"] == phone {
			claimName = claim.GetName()
			spec, ok := claim.Object["spec"].(map[string]interface{})
			if ok {
				ns, ok := spec["namespace"].(string)
				if ok {
					clusterName = ns
				}
			}
			found = true
			break
		}
	}

	// If not found, grab the first unlabeled ClusterClaim and label it
	if !found {
		for _, claim := range claims.Items {
			if !claimMatchesPool(claim.Object, clusterPool) {
				continue
			}
			labels := claim.GetLabels()
			if labels == nil || labels["prelude"] == "" {
				claimName = claim.GetName()
				spec, ok := claim.Object["spec"].(map[string]interface{})
				if ok {
					ns, ok := spec["namespace"].(string)
					if ok {
						clusterName = ns
					}
				}

				// Label the claim with the phone number
				if labels == nil {
					labels = make(map[string]string)
				}
				labels["prelude"] = phone
				claim.SetLabels(labels)

				// Set spec.lifetime = age + configured lifetime
				configuredDuration, err := parseDuration(clusterLifetime)
				if err != nil {
					log.Printf("Error parsing cluster lifetime %q: %v", clusterLifetime, err)
					http.Error(w, "Invalid cluster lifetime configuration", http.StatusInternalServerError)
					return
				}
				age := time.Since(claim.GetCreationTimestamp().Time)
				totalLifetime := age + configuredDuration
				spec["lifetime"] = formatDuration(totalLifetime)
				log.Printf("Cluster claim %s age=%s, configured=%s, setting lifetime=%s", claimName, formatDuration(age), clusterLifetime, formatDuration(totalLifetime))

				_, err = dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).Update(ctx, &claim, metav1.UpdateOptions{})
				if err != nil {
					log.Printf("Error labeling cluster claim %s: %v", claimName, err)
					http.Error(w, "Failed to assign cluster", http.StatusInternalServerError)
					return
				}
				found = true
				break
			}
		}
	}

	if !found || clusterName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "all_clusters_in_use",
		})
		return
	}

	// Get ClusterDeployment to find webConsoleURL
	cd, err := dynClient.Resource(clusterDeploymentGVR).Namespace(clusterName).Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error getting cluster deployment %s: %v", clusterName, err)
		http.Error(w, "Failed to get cluster deployment", http.StatusInternalServerError)
		return
	}

	webConsoleURL := ""
	if status, ok := cd.Object["status"].(map[string]interface{}); ok {
		if url, ok := status["webConsoleURL"].(string); ok {
			webConsoleURL = url
		}
	}

	// Get kubeconfig secret name from ClusterDeployment
	kubeconfigSecretName := ""
	if spec, ok := cd.Object["spec"].(map[string]interface{}); ok {
		if meta, ok := spec["clusterMetadata"].(map[string]interface{}); ok {
			if ref, ok := meta["adminKubeconfigSecretRef"].(map[string]interface{}); ok {
				if name, ok := ref["name"].(string); ok {
					kubeconfigSecretName = name
				}
			}
		}
	}

	if kubeconfigSecretName == "" {
		log.Printf("Could not find kubeconfig secret ref for cluster %s", clusterName)
		http.Error(w, "Failed to find kubeconfig secret", http.StatusInternalServerError)
		return
	}

	// Get the admin kubeconfig secret (used for htpasswd update)
	adminSecret, err := clientset.CoreV1().Secrets(clusterName).Get(ctx, kubeconfigSecretName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error getting admin kubeconfig secret %s/%s: %v", clusterName, kubeconfigSecretName, err)
		http.Error(w, "Failed to get admin kubeconfig", http.StatusInternalServerError)
		return
	}

	adminKubeconfigData := extractKubeconfig(adminSecret)

	// Derive user kubeconfig secret name from admin kubeconfig secret name
	userKubeconfigSecretName := strings.Replace(kubeconfigSecretName, "-admin-kubeconfig", "-user-kubeconfig", 1)
	log.Printf("Looking up user kubeconfig secret %s/%s", clusterName, userKubeconfigSecretName)

	userSecret, err := clientset.CoreV1().Secrets(clusterName).Get(ctx, userKubeconfigSecretName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error getting user kubeconfig secret %s/%s: %v", clusterName, userKubeconfigSecretName, err)
		http.Error(w, "Failed to get user kubeconfig", http.StatusInternalServerError)
		return
	}

	userKubeconfigData := extractKubeconfig(userSecret)

	// Generate htpasswd entry and update the spoke cluster's htpass-secret (using admin kubeconfig)
	if err := updateHtpasswd(adminKubeconfigData, password); err != nil {
		log.Printf("Error updating htpasswd on spoke cluster %s: %v", clusterName, err)
		http.Error(w, "Failed to set cluster admin password", http.StatusInternalServerError)
		return
	}

	// Derive AI console URL by replacing console-openshift-console with data-science-gateway
	aiConsoleURL := strings.Replace(webConsoleURL, "console-openshift-console", "data-science-gateway", 1)

	resp := claimResponse{
		WebConsoleURL: webConsoleURL,
		AIConsoleURL:  aiConsoleURL,
		Kubeconfig:    userKubeconfigData,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}

	log.Printf("Assigned cluster %s (claim: %s) to phone %s", clusterName, claimName, phone)
	_ = fmt.Sprintf("placeholder to use fmt import: %s", claimName)
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

// updateHtpasswd generates an htpasswd entry for "admin" with the given password
// and updates the htpass-secret in openshift-config on the spoke cluster.
func updateHtpasswd(spokeKubeconfig string, password string) error {
	// Generate bcrypt hash (equivalent to: htpasswd -bBc /tmp/htpasswd admin password)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("generating bcrypt hash: %w", err)
	}
	htpasswdEntry := fmt.Sprintf("admin:%s\n", string(hash))
	log.Printf("Generated htpasswd entry for admin user")

	// Build a client for the spoke cluster using its kubeconfig
	spokeConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(spokeKubeconfig))
	if err != nil {
		return fmt.Errorf("building spoke kubeconfig: %w", err)
	}
	log.Printf("Connecting to spoke cluster at %s", spokeConfig.Host)

	spokeClient, err := kubernetes.NewForConfig(spokeConfig)
	if err != nil {
		return fmt.Errorf("creating spoke client: %w", err)
	}

	ctx := context.Background()

	// Try to get the existing htpass-secret, create it if it doesn't exist
	secret, err := spokeClient.CoreV1().Secrets("openshift-config").Get(ctx, "htpass-secret", metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		log.Printf("htpass-secret not found on spoke cluster, creating it")
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "htpass-secret",
				Namespace: "openshift-config",
			},
			Data: map[string][]byte{
				"htpasswd": []byte(htpasswdEntry),
			},
		}
		_, err = spokeClient.CoreV1().Secrets("openshift-config").Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating htpass-secret: %w", err)
		}
		log.Printf("Created htpass-secret on spoke cluster")
		return nil
	} else if err != nil {
		return fmt.Errorf("getting htpass-secret: %w", err)
	}

	// Update the htpasswd data
	log.Printf("Existing htpass-secret found, data keys: %v", secretDataKeys(secret))
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["htpasswd"] = []byte(htpasswdEntry)

	_, err = spokeClient.CoreV1().Secrets("openshift-config").Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating htpass-secret: %w", err)
	}

	log.Printf("Updated htpass-secret on spoke cluster")
	return nil
}

func secretDataKeys(s *corev1.Secret) []string {
	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}
	return keys
}
