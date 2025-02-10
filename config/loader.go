/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

func LoadConfig(c *WorkerConfig) error {
	configFileName := "simi.yaml"
	if fn := os.Getenv("SIMI_YAML"); fn != "" {
		configFileName = fn
	}
	yamlFile, err := os.ReadFile(configFileName)
	if err != nil {
		return fmt.Errorf("error reading %s:  %w", configFileName, err)
	}

	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		return fmt.Errorf("error unmarshaling %s: %w", configFileName, err)
	}

	return nil
}
