package env

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GetEnv reads an environment variable, falls back to _FILE version if not set,
// and as a third check, reads from a default secrets file under /run/secrets/{ENV}.
func GetEnv(key string, defaultValue ...string) string {
	defaultVal := ""
	if len(defaultValue) > 0 {
		defaultVal = defaultValue[0]
	}

	// First, check if the environment variable is directly set
	if value := os.Getenv(key); len(value) != 0 {
		return value
	}

	// If the env var is not set, check the _FILE version
	if filePath := os.Getenv(key + "_FILE"); len(filePath) != 0 {
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("Error reading file for env var %s_FILE: %v", key, err)
			return defaultVal
		}

		if value := strings.TrimSpace(string(content)); len(value) != 0 {
			return value
		}
	}

	// Finally, check /run/secrets/{key}
	if secretsFilePath := filepath.Join("/run/secrets", key); fileExists(secretsFilePath) {
		content, err := os.ReadFile(secretsFilePath)
		if err != nil {
			log.Printf("Error reading secret file for env var %s: %v", secretsFilePath, err)
			return defaultVal
		}

		if value := strings.TrimSpace(string(content)); len(value) != 0 {
			return value
		}
	}

	// If no values found, return the default
	return defaultVal
}

// GetEnvInt64 reads an int64 environment variable and falls back to _FILE version or /run/secrets/{key}
func GetEnvInt64[T int | int64](key string, defaultValue ...T) int64 {
	if valueStr := GetEnv(key, ""); len(valueStr) != 0 {
		if parsed, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return parsed
		}
	}

	if len(defaultValue) > 0 {
		return int64(defaultValue[0])
	}

	return 0
}

func GetEnvInt64Slice(key string, defaultValues ...int64) []int64 {
	if valueStr := GetEnv(key, ""); len(valueStr) != 0 {
		parts := strings.Split(valueStr, ",")
		values := make([]int64, 0, len(parts))

		for _, part := range parts {
			if parsed, err := strconv.ParseInt(part, 10, 64); err == nil {
				values = append(values, parsed)
			}
		}

		return values
	}

	if len(defaultValues) > 0 {
		return defaultValues
	}

	return nil
}

// GetEnvInt reads an int environment variable and falls back to _FILE version or /run/secrets/{key}.
func GetEnvInt(key string, defaultValue ...int) int {
	return int(GetEnvInt64(key, defaultValue...))
}

// fileExists checks if a file exists and is not a directory.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if err != nil {
		return false
	}

	return !info.IsDir()
}
