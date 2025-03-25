package env

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// GetAsString retrieves an environment variable as a string.
// If required is true and the variable is not set, an error is returned.
// If not required and not set, defaultValue is returned.
func GetAsString(key string, required bool, defaultValue string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		if required {
			return "", fmt.Errorf("required environment variable %s is not set", key)
		}
		return defaultValue, nil
	}
	return value, nil
}

// GetAsInt retrieves an environment variable as an integer.
// If required is true and the variable is not set or cannot be parsed as int, an error is returned.
// If not required and not set or invalid, defaultValue is returned.
func GetAsInt(key string, required bool, defaultValue int) (int, error) {
	value, err := GetAsString(key, required, strconv.Itoa(defaultValue))
	if err != nil {
		return 0, err
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		if required {
			return 0, fmt.Errorf("environment variable %s must be an integer: %w", key, err)
		}
		return defaultValue, nil
	}

	return intValue, nil
}

// GetAsBool retrieves an environment variable as a boolean.
// If required is true and the variable is not set or cannot be parsed as bool, an error is returned.
// If not required and not set or invalid, defaultValue is returned.
func GetAsBool(key string, required bool, defaultValue bool) (bool, error) {
	value, err := GetAsString(key, required, strconv.FormatBool(defaultValue))
	if err != nil {
		return false, err
	}

	// Convert to lowercase for easier comparison
	valueLower := strings.ToLower(value)

	// Check for various true/false representations
	switch valueLower {
	case "true", "1", "yes", "y", "on":
		return true, nil
	case "false", "0", "no", "n", "off":
		return false, nil
	default:
		if required {
			return false, fmt.Errorf("environment variable %s must be a boolean value", key)
		}
		return defaultValue, nil
	}
}

// GetAsFloat retrieves an environment variable as a float64.
// If required is true and the variable is not set or cannot be parsed as float, an error is returned.
// If not required and not set or invalid, defaultValue is returned.
func GetAsFloat(key string, required bool, defaultValue float64) (float64, error) {
	value, err := GetAsString(key, required, strconv.FormatFloat(defaultValue, 'f', -1, 64))
	if err != nil {
		return 0, err
	}

	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		if required {
			return 0, fmt.Errorf("environment variable %s must be a number: %w", key, err)
		}
		return defaultValue, nil
	}

	return floatValue, nil
}
