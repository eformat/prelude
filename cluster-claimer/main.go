package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	clusterDeploymentGVR = schema.GroupVersionResource{
		Group:    "hive.openshift.io",
		Version:  "v1",
		Resource: "clusterdeployments",
	}
	clusterClaimGVR = schema.GroupVersionResource{
		Group:    "hive.openshift.io",
		Version:  "v1",
		Resource: "clusterclaims",
	}
	clusterPoolNamespace = "cluster-pools"
)

func main() {
	clusterPool := flag.String("cluster-pool", os.Getenv("CLUSTER_POOL"), "ClusterPool name to filter by (required)")
	clusterClaimLimitStr := flag.String("cluster-claim-limit", os.Getenv("CLUSTER_CLAIM_LIMIT"), "Base number of ClusterClaims to create (default 4)")
	clusterClaimMaxStr := flag.String("cluster-claim-max", os.Getenv("CLUSTER_CLAIM_MAX"), "Maximum number of ClusterClaims when scaling up (default 10)")
	clusterClaimIncrementStr := flag.String("cluster-claim-increment", os.Getenv("CLUSTER_CLAIM_INCREMENT"), "Number of ClusterClaims to add when scaling up (default 1)")
	clusterClaimAvailableThresholdStr := flag.String("cluster-claim-available-threshold", os.Getenv("CLUSTER_CLAIM_AVAILABLE_THRESHOLD"), "Available cluster count at which to trigger scale-up (default 1)")
	flag.Parse()

	if *clusterPool == "" {
		log.Fatalf("--cluster-pool flag or CLUSTER_POOL environment variable is required")
	}

	claimLimit := 4
	if *clusterClaimLimitStr != "" {
		n, err := fmt.Sscanf(*clusterClaimLimitStr, "%d", &claimLimit)
		if n != 1 || err != nil {
			log.Fatalf("Invalid --cluster-claim-limit value: %s", *clusterClaimLimitStr)
		}
	}

	claimMax := 10
	if *clusterClaimMaxStr != "" {
		n, err := fmt.Sscanf(*clusterClaimMaxStr, "%d", &claimMax)
		if n != 1 || err != nil {
			log.Fatalf("Invalid --cluster-claim-max value: %s", *clusterClaimMaxStr)
		}
	}
	claimIncrement := 1
	if *clusterClaimIncrementStr != "" {
		n, err := fmt.Sscanf(*clusterClaimIncrementStr, "%d", &claimIncrement)
		if n != 1 || err != nil {
			log.Fatalf("Invalid --cluster-claim-increment value: %s", *clusterClaimIncrementStr)
		}
	}
	availableThreshold := 1
	if *clusterClaimAvailableThresholdStr != "" {
		n, err := fmt.Sscanf(*clusterClaimAvailableThresholdStr, "%d", &availableThreshold)
		if n != 1 || err != nil {
			log.Fatalf("Invalid --cluster-claim-available-threshold value: %s", *clusterClaimAvailableThresholdStr)
		}
	}

	if claimMax < claimLimit {
		claimMax = claimLimit
	}

	log.Printf("Cluster pool: %s", *clusterPool)
	log.Printf("Cluster claim limit: %d (max: %d, increment: %d, available threshold: %d)", claimLimit, claimMax, claimIncrement, availableThreshold)

	config, err := buildConfig()
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating dynamic client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := *clusterPool

	// Handle shutdown signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("Received shutdown signal")
		cancel()
	}()

	// Step 1: Wait for at least one provisioned ClusterDeployment
	log.Printf("Waiting for cluster pool %s to be provisioned...", pool)
	if err := waitForProvisioned(ctx, dynClient, pool); err != nil {
		log.Fatalf("Error waiting for provisioned: %v", err)
	}

	// Step 2: Reconcile loop — watch for changes and create claims as needed
	reconcile(ctx, dynClient, pool, claimLimit, claimMax, claimIncrement, availableThreshold)
	log.Printf("Cluster claimer shutting down")
}

// reconcile continuously watches ClusterDeployments and creates ClusterClaims
// as new deployments become provisioned, up to the claim limit. The effective
// limit starts at baseLimit and increases when no clusters are available,
// up to maxLimit. It scales back down to baseLimit after clusters have been
// available for 10 minutes (hysteresis).
func reconcile(ctx context.Context, dynClient dynamic.Interface, pool string, baseLimit, maxLimit, increment, availableThreshold int) {
	labelSelector := fmt.Sprintf("hive.openshift.io/clusterpool-name=%s", pool)
	effectiveLimit := baseLimit
	var availableSince time.Time // when available clusters were first seen
	var lastScaleUp time.Time   // when we last scaled up (25min cooldown)

	for {
		if ctx.Err() != nil {
			return
		}

		// Dynamic scaling of effective limit
		available, ready, err := countAvailableAndReadyClaims(ctx, dynClient, pool)
		if err != nil {
			log.Printf("Error counting available claims: %v", err)
		} else if available <= availableThreshold && ready > 0 {
			// Available clusters at or below threshold — scale up (with 25min cooldown) and reset scale-down timer
			availableSince = time.Time{}
			if effectiveLimit < maxLimit {
				if !lastScaleUp.IsZero() && time.Since(lastScaleUp) < 25*time.Minute {
					log.Printf("No available clusters, waiting for previous scale-up to take effect (%s ago)", time.Since(lastScaleUp).Truncate(time.Second))
				} else {
					prev := effectiveLimit
					effectiveLimit += increment
					if effectiveLimit > maxLimit {
						effectiveLimit = maxLimit
					}
					lastScaleUp = time.Now()
					log.Printf("No available clusters, increasing claim limit from %d to %d (max: %d)", prev, effectiveLimit, maxLimit)
				}
			}
		} else {
			// Clusters are available — track for hysteresis and scale down after 10min
			if availableSince.IsZero() {
				availableSince = time.Now()
				log.Printf("Available clusters detected (%d), starting hysteresis timer", available)
			} else if effectiveLimit > baseLimit && time.Since(availableSince) >= 10*time.Minute {
				log.Printf("Clusters available for 10+ minutes, scaling claim limit back from %d to %d", effectiveLimit, baseLimit)
				effectiveLimit = baseLimit
				availableSince = time.Time{}
			}
		}

		// Check and create any needed claims
		created := createNeededClaims(ctx, dynClient, pool, effectiveLimit)
		if created > 0 {
			log.Printf("Reconcile: created %d claim(s)", created)
		}

		// Watch for ClusterDeployment changes, then re-reconcile
		var timeoutSecs int64 = 30
		list, err := dynClient.Resource(clusterDeploymentGVR).Namespace("").List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			log.Printf("Error listing ClusterDeployments: %v", err)
			sleepOrDone(ctx, 10*time.Second)
			continue
		}

		watcher, err := dynClient.Resource(clusterDeploymentGVR).Namespace("").Watch(ctx, metav1.ListOptions{
			LabelSelector:   labelSelector,
			TimeoutSeconds:  &timeoutSecs,
			ResourceVersion: list.GetResourceVersion(),
		})
		if err != nil {
			log.Printf("Error watching ClusterDeployments: %v", err)
			sleepOrDone(ctx, 10*time.Second)
			continue
		}

		for event := range watcher.ResultChan() {
			if event.Type == watch.Added || event.Type == watch.Modified {
				if u, ok := event.Object.(*unstructured.Unstructured); ok {
					if isProvisioned(u.Object) {
						log.Printf("ClusterDeployment %s/%s changed, re-reconciling", u.GetNamespace(), u.GetName())
						break
					}
				}
			}
		}
		watcher.Stop()
	}
}

// createNeededClaims checks how many claims are needed and creates them.
// Returns the number of claims created.
func createNeededClaims(ctx context.Context, dynClient dynamic.Interface, pool string, claimLimit int) int {
	needed, err := claimsNeeded(ctx, dynClient, pool, claimLimit)
	if err != nil {
		log.Printf("Error determining claims needed: %v", err)
		return 0
	}
	if needed == 0 {
		return 0
	}

	existingNames, err := existingClaimNames(ctx, dynClient, pool)
	if err != nil {
		log.Printf("Error listing existing claim names: %v", err)
		return 0
	}

	created := 0
	for i := 1; created < needed; i++ {
		name := fmt.Sprintf("prelude%d", i)
		if existingNames[name] {
			continue
		}
		log.Printf("Creating ClusterClaim %s for pool %s", name, pool)
		if err := createClusterClaim(ctx, dynClient, name, pool); err != nil {
			log.Printf("Error creating cluster claim: %v", err)
			return created
		}
		created++
	}
	return created
}

// sleepOrDone sleeps for the given duration or returns early if the context is cancelled.
func sleepOrDone(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// claimsNeeded returns how many new ClusterClaims are needed by comparing
// the number of provisioned ClusterDeployments to existing ClusterClaims for the pool,
// capped by the cluster claim limit.
func claimsNeeded(ctx context.Context, dynClient dynamic.Interface, pool string, claimLimit int) (int, error) {
	provisionedCount, err := countProvisionedDeployments(ctx, dynClient, pool)
	if err != nil {
		return 0, err
	}

	claimCount, err := countClaimsForPool(ctx, dynClient, pool)
	if err != nil {
		return 0, err
	}

	log.Printf("Provisioned ClusterDeployments: %d, existing ClusterClaims: %d, claim limit: %d", provisionedCount, claimCount, claimLimit)

	// Cap the target number of claims at the limit
	target := provisionedCount
	if target > claimLimit {
		target = claimLimit
	}

	needed := target - claimCount
	if needed < 0 {
		needed = 0
	}
	return needed, nil
}

// countProvisionedDeployments counts ClusterDeployments with the Provisioned condition
// that belong to the specified cluster pool.
func countProvisionedDeployments(ctx context.Context, dynClient dynamic.Interface, pool string) (int, error) {
	labelSelector := fmt.Sprintf("hive.openshift.io/clusterpool-name=%s", pool)
	list, err := dynClient.Resource(clusterDeploymentGVR).Namespace("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return 0, fmt.Errorf("listing ClusterDeployments: %w", err)
	}

	count := 0
	for _, cd := range list.Items {
		if isProvisioned(cd.Object) {
			count++
		}
	}
	return count, nil
}

// countClaimsForPool counts existing ClusterClaims that reference the specified pool.
func countClaimsForPool(ctx context.Context, dynClient dynamic.Interface, pool string) (int, error) {
	claims, err := dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("listing ClusterClaims: %w", err)
	}

	count := 0
	for _, claim := range claims.Items {
		if claimMatchesPool(claim.Object, pool) {
			count++
		}
	}
	return count, nil
}

// countAvailableAndReadyClaims counts ClusterClaims that are authenticated (prelude-auth=done)
// but not yet claimed by a user (no prelude phone label), and also returns the total
// number of ready (authenticated) clusters including claimed ones.
func countAvailableAndReadyClaims(ctx context.Context, dynClient dynamic.Interface, pool string) (available int, ready int, err error) {
	claims, err := dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("listing ClusterClaims: %w", err)
	}

	for _, claim := range claims.Items {
		if !claimMatchesPool(claim.Object, pool) {
			continue
		}
		labels := claim.GetLabels()
		if labels["prelude-auth"] == "done" {
			ready++
			if labels["prelude"] == "" {
				available++
			}
		}
	}
	return available, ready, nil
}

// existingClaimNames returns the set of ClusterClaim names that already exist for the pool.
func existingClaimNames(ctx context.Context, dynClient dynamic.Interface, pool string) (map[string]bool, error) {
	claims, err := dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing ClusterClaims: %w", err)
	}

	names := make(map[string]bool)
	for _, claim := range claims.Items {
		if claimMatchesPool(claim.Object, pool) {
			names[claim.GetName()] = true
		}
	}
	return names, nil
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

// waitForProvisioned watches ClusterDeployments matching the cluster pool label
// and waits until at least one has the Provisioned condition set to True.
func waitForProvisioned(ctx context.Context, dynClient dynamic.Interface, pool string) error {
	labelSelector := fmt.Sprintf("hive.openshift.io/clusterpool-name=%s", pool)
	timeout := 100 * time.Minute
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check current state
		list, err := dynClient.Resource(clusterDeploymentGVR).Namespace("").List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			log.Printf("Error listing ClusterDeployments: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		for _, cd := range list.Items {
			if isProvisioned(cd.Object) {
				log.Printf("ClusterDeployment %s/%s is provisioned", cd.GetNamespace(), cd.GetName())
				return nil
			}
		}

		// Watch for changes with a 30s timeout
		var timeoutSecs int64 = 30
		watcher, err := dynClient.Resource(clusterDeploymentGVR).Namespace("").Watch(ctx, metav1.ListOptions{
			LabelSelector:   labelSelector,
			TimeoutSeconds:  &timeoutSecs,
			ResourceVersion: list.GetResourceVersion(),
		})
		if err != nil {
			log.Printf("Error watching ClusterDeployments: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		provisioned := false
		for event := range watcher.ResultChan() {
			if event.Type == watch.Added || event.Type == watch.Modified {
				if u, ok := event.Object.(*unstructured.Unstructured); ok {
					if isProvisioned(u.Object) {
						log.Printf("ClusterDeployment %s/%s is now provisioned", u.GetNamespace(), u.GetName())
						provisioned = true
						break
					}
				}
			}
		}
		watcher.Stop()

		if provisioned {
			return nil
		}

		log.Printf("Waiting for cluster pool %s to be provisioned...", pool)
	}

	return fmt.Errorf("timed out waiting for cluster pool %s to be provisioned after %v", pool, timeout)
}

// isProvisioned checks if a ClusterDeployment has the Provisioned condition set to True.
func isProvisioned(obj map[string]interface{}) bool {
	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return false
	}
	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return false
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Provisioned" && cond["status"] == "True" {
			return true
		}
	}
	return false
}

// createClusterClaim creates a ClusterClaim resource in the cluster-pools namespace.
func createClusterClaim(ctx context.Context, dynClient dynamic.Interface, name, pool string) error {
	claim := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterClaim",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": clusterPoolNamespace,
			},
			"spec": map[string]interface{}{
				"clusterPoolName": pool,
				"subjects": []interface{}{
					map[string]interface{}{
						"kind":     "Group",
						"apiGroup": "rbac.authorization.k8s.io",
						"name":     "system:masters",
					},
				},
			},
		},
	}

	_, err := dynClient.Resource(clusterClaimGVR).Namespace(clusterPoolNamespace).Create(ctx, claim, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating ClusterClaim %s: %w", name, err)
	}
	log.Printf("ClusterClaim %s created successfully", name)
	return nil
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
