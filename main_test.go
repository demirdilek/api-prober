package main

import (
	"os"
	"path/filepath"
	"testing"
)

// English comments as requested

func TestGetEnvAsInt(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envVal       string
		defaultVal   int
		expectedVal  int
		shouldSetEnv bool
	}{
		{
			name:         "Default value when env is empty",
			envKey:       "TEST_WORKERS_EMPTY",
			envVal:       "",
			defaultVal:   50,
			expectedVal:  50,
			shouldSetEnv: false,
		},
		{
			name:         "Valid integer from env",
			envKey:       "TEST_WORKERS_VALID",
			envVal:       "100",
			defaultVal:   50,
			expectedVal:  100,
			shouldSetEnv: true,
		},
		{
			name:         "Fallback to default on invalid string",
			envKey:       "TEST_WORKERS_INVALID",
			envVal:       "invalid_int",
			defaultVal:   50,
			expectedVal:  50,
			shouldSetEnv: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldSetEnv {
				t.Setenv(tt.envKey, tt.envVal)
			} else {
				os.Unsetenv(tt.envKey)
			}

			got := getEnvAsInt(tt.envKey, tt.defaultVal)
			if got != tt.expectedVal {
				t.Errorf("getEnvAsInt(%s, %d) = %d; want %d", tt.envKey, tt.defaultVal, got, tt.expectedVal)
			}
		})
	}
}

func TestInitKubeClient_KubeconfigFallback(t *testing.T) {
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

	clientset := initKubeClient()
	if clientset == nil {
		t.Fatal("expected non-nil kubernetes Clientset")
	}
}