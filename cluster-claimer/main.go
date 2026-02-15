package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
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
	clusterPoolGVR = schema.GroupVersionResource{
		Group:    "hive.openshift.io",
		Version:  "v1",
		Resource: "clusterpools",
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

	ctx := context.Background()
	pool := *clusterPool

	// Step 1: Watch/wait for ClusterDeployments to be provisioned
	log.Printf("Waiting for cluster pool %s to be provisioned...", pool)
	if err := waitForProvisioned(ctx, dynClient, pool); err != nil {
		log.Fatalf("Error waiting for provisioned: %v", err)
	}

	// Step 2: Determine how many claims are needed
	needed, err := claimsNeeded(ctx, dynClient, pool)
	if err != nil {
		log.Fatalf("Error determining claims needed: %v", err)
	}
	if needed == 0 {
		log.Printf("All provisioned ClusterDeployments already have ClusterClaims, nothing to do")
		select {}
	}

	// Step 3: Create ClusterClaims with generated names (prelude1, prelude2, ...)
	existingNames, err := existingClaimNames(ctx, dynClient, pool)
	if err != nil {
		log.Fatalf("Error listing existing claim names: %v", err)
	}
	created := 0
	for i := 1; created < needed; i++ {
		name := fmt.Sprintf("prelude%d", i)
		if existingNames[name] {
			continue
		}
		log.Printf("Creating ClusterClaim %s for pool %s", name, pool)
		if err := createClusterClaim(ctx, dynClient, name, pool); err != nil {
			log.Fatalf("Error creating cluster claim: %v", err)
		}
		created++
	}

	log.Printf("Cluster claimer completed successfully, created %d claim(s)", created)
	select {} // idle to keep container alive
}

// claimsNeeded returns how many new ClusterClaims are needed by comparing
// the number of provisioned ClusterDeployments to existing ClusterClaims for the pool.
func claimsNeeded(ctx context.Context, dynClient dynamic.Interface, pool string) (int, error) {
	provisionedCount, err := countProvisionedDeployments(ctx, dynClient, pool)
	if err != nil {
		return 0, err
	}

	claimCount, err := countClaimsForPool(ctx, dynClient, pool)
	if err != nil {
		return 0, err
	}

	log.Printf("Provisioned ClusterDeployments: %d, existing ClusterClaims: %d", provisionedCount, claimCount)

	needed := provisionedCount - claimCount
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
