package env

import (
	"os"
	"testing"
)


func TestGetInt(t *testing.T) {
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

			got := GetInt(tt.envKey, tt.defaultVal)
			if got != tt.expectedVal {
				t.Errorf("getEnvAsInt(%s, %d) = %d; want %d", tt.envKey, tt.defaultVal, got, tt.expectedVal)
			}
		})
	}
}

