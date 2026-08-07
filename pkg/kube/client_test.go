package kube

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitClient_KubeconfigFallback(t *testing.T) {
	// Point KUBECONFIG to a non-existent file to ensure clientcmd handles the path resolution
	tempDir := t.TempDir()
	fakeKubeconfig := filepath.Join(tempDir, "config")
	t.Setenv("KUBECONFIG", fakeKubeconfig)

	// Create a dummy minimal kubeconfig file
	dummyConfig := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: http://127.0.0.1:8080
  name: dummy
contexts:
- context:
    cluster: dummy
    user: dummy
  name: dummy
current-context: dummy
users:
- name: dummy
`
	if err := os.WriteFile(fakeKubeconfig, []byte(dummyConfig), 0600); err != nil {
		t.Fatalf("failed to write dummy kubeconfig: %v", err)
	}

	clientset := InitClient()
	if clientset == nil {
		t.Fatal("expected non-nil kubernetes Clientset")
	}
}