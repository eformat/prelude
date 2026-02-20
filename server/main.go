package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	mathrand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	clusterPoolNamespace  = "cluster-pools"
	recaptchaVerifyURL    = "https://www.google.com/recaptcha/api/siteverify"
	recaptchaMinScore     = 0.5
)

var recaptchaSecretKey string
var recaptchaSiteKey string

var adminPassword string
var adminTokens = struct {
	sync.RWMutex
	m map[string]bool
}{m: make(map[string]bool)}

type adminLoginRequest struct {
	Password string `json:"password"`
}

type claimRequest struct {
	Phone          string `json:"phone"`
	Password       string `json:"password"`
	RecaptchaToken string `json:"recaptchaToken"`
	Fingerprint    string `json:"fingerprint"`
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

// sanitizeFingerprint validates and truncates a browser fingerprint to hex chars only, max 16 chars.
func sanitizeFingerprint(fp string) string {
	var b strings.Builder
	for _, r := range fp {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			b.WriteRune(r)
		}
		if b.Len() >= 16 {
			break
		}
	}
	return b.String()
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

// formatDuration formats a duration using h, m units (no days, since Kubernetes
// duration fields only accept standard Go duration units: ns, us, ms, s, m, h).
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	var parts []string
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
	ExpiresAt     string `json:"expiresAt"`
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

	adminPassword = os.Getenv("ADMIN_PASSWORD")
	if adminPassword != "" {
		log.Printf("Admin page authentication enabled")
	} else {
		log.Printf("Admin page authentication disabled (ADMIN_PASSWORD not set)")
	}

	log.Printf("Filtering ClusterClaims by clusterPoolName: %s", *clusterPool)
	log.Printf("Cluster lifetime: %s", *clusterLifetime)

	config, err := buildConfig()
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
	mux.HandleFunc("/api/cluster/ready", func(w http.ResponseWriter, r *http.Request) {
		handleClusterReady(w, r, dynClient, clientset, pool)
	})
	mux.HandleFunc("/api/admin/login", handleAdminLogin)
	mux.HandleFunc("/api/admin", func(w http.ResponseWriter, r *http.Request) {
		handleAdmin(w, r, dynClient, pool)
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

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func validateAdminToken(r *http.Request) bool {
	if adminPassword == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	adminTokens.RLock()
	defer adminTokens.RUnlock()
	return adminTokens.m[token]
}

func handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if adminPassword == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": ""})
		return
	}

	var req adminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Password != adminPassword {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	token, err := generateToken()
	if err != nil {
		log.Printf("Error generating admin token: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	adminTokens.Lock()
	adminTokens.m[token] = true
	adminTokens.Unlock()

	log.Printf("Admin login successful, token issued")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

type adminClaimInfo struct {
	Name          string `json:"name"`
	Pool          string `json:"pool"`
	Phone         string `json:"phone"`
	Authenticated bool   `json:"authenticated"`
	Namespace     string `json:"namespace"`
	Age           string `json:"age"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
}

type adminDeploymentInfo struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Platform        string `json:"platform"`
	Region          string `json:"region"`
	Version         string `json:"version"`
	ProvisionStatus string `json:"provisionStatus"`
	PowerState      string `json:"powerState"`
	Age             string `json:"age"`
}

type adminResponse struct {
	ClusterClaims      []adminClaimInfo      `json:"clusterClaims"`
	ClusterDeployments []adminDeploymentInfo `json:"clusterDeployments"`
}

func handleAdmin(w http.ResponseWriter, r *http.Request, dynClient dynamic.Interface, pool string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !validateAdminToken(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := context.Background()

	// List ClusterClaims
	claims, err := dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Admin: error listing ClusterClaims: %v", err)
		http.Error(w, "Failed to list cluster claims", http.StatusInternalServerError)
		return
	}

	var claimInfos []adminClaimInfo
	for _, claim := range claims.Items {
		if !claimMatchesPool(claim.Object, pool) {
			continue
		}
		labels := claim.GetLabels()
		phone := ""
		authenticated := false
		if labels != nil {
			phone = labels["prelude"]
			authenticated = labels["prelude-auth"] == "done"
		}
		ns := ""
		expiresAt := ""
		if spec, ok := claim.Object["spec"].(map[string]interface{}); ok {
			if v, ok := spec["namespace"].(string); ok {
				ns = v
			}
			if phone != "" {
				if lt, ok := spec["lifetime"].(string); ok {
					if d, err := parseDuration(lt); err == nil {
						expiresAt = claim.GetCreationTimestamp().Time.Add(d).UTC().Format(time.RFC3339)
					}
				}
			}
		}
		age := formatAge(time.Since(claim.GetCreationTimestamp().Time))
		claimInfos = append(claimInfos, adminClaimInfo{
			Name:          claim.GetName(),
			Pool:          pool,
			Phone:         phone,
			Authenticated: authenticated,
			Namespace:     ns,
			Age:           age,
			ExpiresAt:     expiresAt,
		})
	}

	// List ClusterDeployments across all namespaces filtered by pool label
	deployments, err := dynClient.Resource(clusterDeploymentGVR).Namespace("").List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("hive.openshift.io/clusterpool-name=%s", pool),
	})
	if err != nil {
		log.Printf("Admin: error listing ClusterDeployments: %v", err)
		http.Error(w, "Failed to list cluster deployments", http.StatusInternalServerError)
		return
	}

	var deployInfos []adminDeploymentInfo
	for _, cd := range deployments.Items {
		platform := ""
		region := ""
		version := ""
		if spec, ok := cd.Object["spec"].(map[string]interface{}); ok {
			if p, ok := spec["platform"].(map[string]interface{}); ok {
				for k, v := range p {
					if pm, ok := v.(map[string]interface{}); ok {
						platform = k
						if r, ok := pm["region"].(string); ok {
							region = r
						}
						break
					}
				}
			}
		}

		provisionStatus := ""
		powerState := ""
		if status, ok := cd.Object["status"].(map[string]interface{}); ok {
			if conditions, ok := status["conditions"].([]interface{}); ok {
				for _, c := range conditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					condType, _ := cond["type"].(string)
					condStatus, _ := cond["status"].(string)
					if condType == "Provisioned" && condStatus == "True" {
						provisionStatus = "Provisioned"
					}
					if condType == "Provisioning" && condStatus == "True" && provisionStatus == "" {
						provisionStatus = "Provisioning"
					}
				}
			}
			if ps, ok := status["powerState"].(string); ok {
				powerState = ps
			}
			if v, ok := status["installVersion"].(string); ok {
				version = v
			}
		}

		age := formatAge(time.Since(cd.GetCreationTimestamp().Time))
		deployInfos = append(deployInfos, adminDeploymentInfo{
			Name:            cd.GetName(),
			Namespace:       cd.GetNamespace(),
			Platform:        platform,
			Region:          region,
			Version:         version,
			ProvisionStatus: provisionStatus,
			PowerState:      powerState,
			Age:             age,
		})
	}

	resp := adminResponse{
		ClusterClaims:      claimInfos,
		ClusterDeployments: deployInfos,
	}
	if resp.ClusterClaims == nil {
		resp.ClusterClaims = []adminClaimInfo{}
	}
	if resp.ClusterDeployments == nil {
		resp.ClusterDeployments = []adminDeploymentInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// formatAge formats a duration as a human-readable age string (e.g. "67m", "2h30m", "1d3h").
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", minutes)
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

	fingerprint := sanitizeFingerprint(req.Fingerprint)

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
	var expiresAt time.Time
	found := false

	// Check if any ClusterClaim already has this phone number
	// Only consider claims that have been authenticated (prelude-auth=done)
	for _, claim := range claims.Items {
		if !claimMatchesPool(claim.Object, clusterPool) {
			continue
		}
		labels := claim.GetLabels()
		if labels == nil || labels["prelude-auth"] != "done" {
			continue
		}
		if labels["prelude"] == phone {
			claimName = claim.GetName()
			spec, ok := claim.Object["spec"].(map[string]interface{})
			if ok {
				ns, ok := spec["namespace"].(string)
				if ok {
					clusterName = ns
				}
			}
			// Compute expiry from existing spec.lifetime
			if spec != nil {
				if lt, ok := spec["lifetime"].(string); ok {
					if d, err := parseDuration(lt); err == nil {
						expiresAt = claim.GetCreationTimestamp().Time.Add(d)
					}
				}
			}
			// Backfill fingerprint label if not already set
			if fingerprint != "" && labels["prelude-fp"] != fingerprint {
				labels["prelude-fp"] = fingerprint
				claim.SetLabels(labels)
				if _, err := dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).Update(ctx, &claim, metav1.UpdateOptions{}); err != nil {
					log.Printf("Warning: failed to backfill fingerprint on claim %s: %v", claimName, err)
				} else {
					log.Printf("Backfilled fingerprint %s on claim %s", fingerprint, claimName)
				}
			}
			found = true
			break
		}
	}

	// If phone not found, check if this fingerprint already claimed a different cluster
	if !found && fingerprint != "" {
		for _, claim := range claims.Items {
			if !claimMatchesPool(claim.Object, clusterPool) {
				continue
			}
			labels := claim.GetLabels()
			if labels == nil || labels["prelude-auth"] != "done" {
				continue
			}
			if labels["prelude-fp"] == fingerprint && labels["prelude"] != "" && labels["prelude"] != phone {
				log.Printf("Fingerprint %s already claimed by phone %s, rejecting phone %s", fingerprint, labels["prelude"], phone)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "device_already_claimed",
				})
				return
			}
		}
	}

	// If not found, pick a random authenticated but unclaimed ClusterClaim and label it
	if !found {
		// Collect all available (authenticated, unclaimed) claim indices
		var availableIndices []int
		for i, claim := range claims.Items {
			if !claimMatchesPool(claim.Object, clusterPool) {
				continue
			}
			labels := claim.GetLabels()
			if labels == nil || labels["prelude-auth"] != "done" {
				continue
			}
			if labels["prelude"] == "" {
				availableIndices = append(availableIndices, i)
			}
		}

		if len(availableIndices) > 0 {
			// Pick a random available claim
			idx := availableIndices[mathrand.IntN(len(availableIndices))]
			claim := claims.Items[idx]
			labels := claim.GetLabels()

			claimName = claim.GetName()
			spec, ok := claim.Object["spec"].(map[string]interface{})
			if ok {
				ns, ok := spec["namespace"].(string)
				if ok {
					clusterName = ns
				}
			}

			// Label the claim with the phone number and fingerprint
			labels["prelude"] = phone
			if fingerprint != "" {
				labels["prelude-fp"] = fingerprint
			}
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
			expiresAt = claim.GetCreationTimestamp().Time.Add(totalLifetime)
			log.Printf("Cluster claim %s age=%s, configured=%s, setting lifetime=%s (picked randomly from %d available)", claimName, formatDuration(age), clusterLifetime, formatDuration(totalLifetime), len(availableIndices))

			_, err = dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).Update(ctx, &claim, metav1.UpdateOptions{})
			if err != nil {
				log.Printf("Error labeling cluster claim %s: %v", claimName, err)
				http.Error(w, "Failed to assign cluster", http.StatusInternalServerError)
				return
			}
			found = true
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

		// Spoke cluster is unreachable (likely deprovisioning) — remove labels so it's
		// no longer considered available and the user can get a different cluster.
		log.Printf("Removing prelude and prelude-auth labels from claim %s (cluster unreachable)", claimName)
		unlabelClaim(ctx, dynClient, claimName)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "cluster_unavailable",
		})
		return
	}

	// Derive AI console URL by replacing console-openshift-console with data-science-gateway
	aiConsoleURL := strings.Replace(webConsoleURL, "console-openshift-console", "data-science-gateway", 1) + "/learning-resources?&keyword=prelude"

	resp := claimResponse{
		WebConsoleURL: webConsoleURL,
		AIConsoleURL:  aiConsoleURL,
		Kubeconfig:    userKubeconfigData,
		ExpiresAt:     expiresAt.UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}

	log.Printf("Assigned cluster %s (claim: %s) to phone %s", clusterName, claimName, phone)
	_ = fmt.Sprintf("placeholder to use fmt import: %s", claimName)
}

// handleClusterReady checks if the authentication ClusterOperator on the spoke
// cluster has Progressing=False, indicating the htpasswd identity provider is ready.
func handleClusterReady(w http.ResponseWriter, r *http.Request, dynClient dynamic.Interface, clientset kubernetes.Interface, clusterPool string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	phone := sanitizePhone(r.URL.Query().Get("phone"))
	if phone == "" {
		http.Error(w, "Phone number is required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Find the ClusterClaim with this phone label
	claims, err := dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Error listing cluster claims for ready check: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ready": false})
		return
	}

	var clusterName string
	for _, claim := range claims.Items {
		if !claimMatchesPool(claim.Object, clusterPool) {
			continue
		}
		labels := claim.GetLabels()
		if labels == nil {
			continue
		}
		if labels["prelude"] == phone {
			clusterName = getSpecNamespace(claim.Object)
			break
		}
	}

	if clusterName == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ready": false})
		return
	}

	// Get ClusterDeployment to find admin kubeconfig
	cd, err := dynClient.Resource(clusterDeploymentGVR).Namespace(clusterName).Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error getting ClusterDeployment for ready check: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ready": false})
		return
	}

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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ready": false})
		return
	}

	adminSecret, err := clientset.CoreV1().Secrets(clusterName).Get(ctx, kubeconfigSecretName, metav1.GetOptions{})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ready": false})
		return
	}

	spokeKubeconfig := extractKubeconfig(adminSecret)
	if spokeKubeconfig == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ready": false})
		return
	}

	spokeConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(spokeKubeconfig))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ready": false})
		return
	}

	spokeDynClient, err := dynamic.NewForConfig(spokeConfig)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ready": false})
		return
	}

	// Check if the authentication ClusterOperator has Progressing=False
	authCO, err := spokeDynClient.Resource(clusterOperatorGVR).Get(ctx, "authentication", metav1.GetOptions{})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ready": false})
		return
	}

	ready := false
	if status, ok := authCO.Object["status"].(map[string]interface{}); ok {
		if conditions, ok := status["conditions"].([]interface{}); ok {
			for _, c := range conditions {
				cond, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				condType, _ := cond["type"].(string)
				condStatus, _ := cond["status"].(string)
				if condType == "Progressing" && condStatus == "False" {
					ready = true
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ready": ready})
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

// unlabelClaim removes the prelude, prelude-auth, and prelude-fp labels from a ClusterClaim,
// making it unavailable for assignment. Used when the spoke cluster is unreachable.
func unlabelClaim(ctx context.Context, dynClient dynamic.Interface, claimName string) {
	claim, err := dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error fetching claim %s for unlabeling: %v", claimName, err)
		return
	}
	labels := claim.GetLabels()
	delete(labels, "prelude")
	delete(labels, "prelude-auth")
	delete(labels, "prelude-fp")
	claim.SetLabels(labels)
	_, err = dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).Update(ctx, claim, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("Error unlabeling claim %s: %v", claimName, err)
		return
	}
	log.Printf("Unlabeled claim %s (removed prelude, prelude-auth, prelude-fp)", claimName)
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

	// Check if the existing password already matches
	log.Printf("Existing htpass-secret found, data keys: %v", secretDataKeys(secret))
	if existing, ok := secret.Data["htpasswd"]; ok {
		// htpasswd format is "admin:<hash>\n", extract the hash
		if parts := strings.SplitN(strings.TrimSpace(string(existing)), ":", 2); len(parts) == 2 {
			if bcrypt.CompareHashAndPassword([]byte(parts[1]), []byte(password)) == nil {
				log.Printf("htpass-secret already has matching password, skipping update")
				return nil
			}
		}
	}

	// Update the htpasswd data
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
