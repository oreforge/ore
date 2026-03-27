package spec

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)}`)

func Load(path string) (*NetworkSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec: %w", err)
	}

	expanded, err := expandEnvVars(string(data))
	if err != nil {
		return nil, err
	}

	var s NetworkSpec
	if err := yaml.Unmarshal([]byte(expanded), &s); err != nil {
		return nil, fmt.Errorf("parsing spec: %w", err)
	}

	if err := Validate(&s); err != nil {
		return nil, err
	}

	return &s, nil
}

func expandEnvVars(input string) (string, error) {
	var expandErr error
	result := envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		if expandErr != nil {
			return match
		}
		varName := envVarPattern.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv(varName)
		if !ok {
			expandErr = fmt.Errorf("environment variable %q is not set (referenced in ore.yaml)", varName)
			return match
		}
		return val
	})
	if expandErr != nil {
		return "", expandErr
	}
	return result, nil
}
