// Package dirconfig resolves the convention-based configuration files shared
// by confmaker and confmaker/confx.
package dirconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const environmentVariable = "ENVIRONMENT"

// Resolve returns the configuration files for ENVIRONMENT in merge order:
// optional common.toml first, then the required environment-specific file.
func Resolve(dir string) ([]string, error) {
	environment, err := resolveEnvironment()
	if err != nil {
		return nil, err
	}

	if dir == "" {
		dir = "."
	}

	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve config directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("resolve config directory %q: not a directory", dir)
	}

	paths := make([]string, 0, 2)
	commonPath := filepath.Join(dir, "common.toml")
	commonExists, err := inspectRegularFile(commonPath, false)
	if err != nil {
		return nil, err
	}
	if commonExists {
		paths = append(paths, commonPath)
	}

	environmentPath := filepath.Join(dir, environment+".toml")
	if _, err := inspectRegularFile(environmentPath, true); err != nil {
		return nil, err
	}
	paths = append(paths, environmentPath)

	return paths, nil
}

func resolveEnvironment() (string, error) {
	raw, ok := os.LookupEnv(environmentVariable)
	if !ok {
		return "", fmt.Errorf(
			"resolve config environment: %s is not set; expected dev, stage, prod, or prd",
			environmentVariable,
		)
	}

	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "dev", "stage", "prod":
		return value, nil
	case "prd":
		return "prod", nil
	default:
		return "", fmt.Errorf(
			"resolve config environment: unsupported %s %q; expected dev, stage, prod, or prd",
			environmentVariable,
			raw,
		)
	}
}

func inspectRegularFile(path string, required bool) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if !required && os.IsNotExist(err) {
			return false, nil
		}

		if required && os.IsNotExist(err) {
			return false, fmt.Errorf("environment config %q does not exist", path)
		}

		return false, fmt.Errorf("inspect config file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("config file %q is not a regular file", path)
	}

	return true, nil
}
