/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

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

func LoadByteSize(s string, c *WorkerConfig) error {
	// Trim and normalize the input
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`(?i)^([\d.]+)\s*([kmgtpezy]?i?b?)$`)
	matches := re.FindStringSubmatch(s)
	if len(matches) != 3 {
		return fmt.Errorf("invalid size format %s", s)
	}

	// Parse number
	numStr := matches[1]
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return err
	}

	// Unit multipliers
	units := map[string]float64{
		"b":   1,
		"kb":  1e3,
		"mb":  1e6,
		"gb":  1e9,
		"tb":  1e12,
		"pb":  1e15,
		"eb":  1e18,
		"zb":  1e21,
		"yb":  1e24,
		"kib": 1 << 10,
		"mib": 1 << 20,
		"gib": 1 << 30,
		"tib": 1 << 40,
		"pib": 1 << 50,
		"eib": 1 << 60,
	}

	unit := strings.ToLower(matches[2])
	if unit == "" {
		unit = "b"
	}

	multiplier, ok := units[unit]
	if !ok {
		return errors.New("unknown size unit: " + unit)
	}

	c.DataSizeBytes = int(num * multiplier)

	return nil
}
