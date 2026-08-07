package env

import (
	"os"
	"strconv"
)

// GetInt reads an environment variable and parses it as an integer.
// Returns the defaultVal if the variable is empty or invalid.
func GetInt(name string, defaultVal int) int {	
	valStr := os.Getenv(name)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}