// Copyright (c) 2023 Cisco and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package gotestmd contains roots command of gotestmd
package gotestmd

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/networkservicemesh/gotestmd/internal/config"
	"github.com/networkservicemesh/gotestmd/internal/generator"
	"github.com/networkservicemesh/gotestmd/internal/linker"
	"github.com/networkservicemesh/gotestmd/internal/parser"
)

// New creates new cmd/gotestmd
func New() *cobra.Command {
	gotestmdCmd := &cobra.Command{
		Use:     "gotestmd",
		Short:   "Command for generating integration tests",
		Version: "0.0.1",

		RunE: func(cmd *cobra.Command, args []string) error {
			match := cmd.Flag("match").Value.String()
			bash := false
			if value, err := cmd.Flags().GetBool("bash"); err == nil {
				bash = value
			}

			if bash && match == "" {
				return errors.New("Flag --bash can be used only with flag --match")
			}

			c := config.FromArgs(args)
			c.Bash = bash
			c.Match = match
			_ = os.MkdirAll(c.OutputDir, os.ModePerm)
			var examples []*parser.Example

			var p = parser.New()
			var l = linker.New(c.InputDir)
			var g = generator.New(c)
			dirs := getRecursiveDirectories(c.InputDir)
			for _, dir := range dirs {
				ex, err := p.ParseFile(path.Join(dir, "README.md"))
				if err == nil {
					examples = append(examples, ex)
				}
			}
			linkedExamples, err := l.Link(examples...)
			if err != nil {
				return errors.Errorf("cannot build examples: %v", err.Error())
			}

			suites := g.Generate(linkedExamples...)

			if !bash {
				return processGoSuites(suites)
			}

			matchRegex, err := regexp.Compile(match)
			if err != nil {
				return err
			}

			return processBashSuites(suites, matchRegex)
		},
	}

	gotestmdCmd.Flags().Bool("bash", false, "generates bash scripts for tests. Can be used only with --match flag")
	gotestmdCmd.Flags().String("match", "", "regex for matching suite or test name. Can be used only with --bash flag")

	return gotestmdCmd
}

func processGoSuites(suites []*generator.Suite) error {
	for _, suite := range suites {
		dir, _ := filepath.Split(suite.Location)
		_ = os.MkdirAll(dir, os.ModePerm)
		err := os.WriteFile(suite.Location, []byte(suite.String()), os.ModePerm)
		if err != nil {
			return errors.Errorf("cannot save suite %v, : %v", suite.Name(), err.Error())
		}
	}

	return nil
}

func processBashSuites(suites []*generator.Suite, matchRegex *regexp.Regexp) error {
	matchFound := false

	for _, suite := range suites {
		if !matchRegex.MatchString(suite.Name()) {
			continue
		}
		matchFound = true
		suite.Tests = nil
		dir, _ := filepath.Split(suite.Location)
		_ = os.MkdirAll(dir, os.ModePerm)
		err := os.WriteFile(suite.Location, []byte(suite.BashString()), os.ModePerm)
		if err != nil {
			return errors.Errorf("cannot save suite %v, : %v", suite.Name(), err.Error())
		}
	}

	for _, suite := range suites {
		matchedTests := make([]*generator.Test, 0)
		for _, test := range suite.Tests {
			if matchRegex.MatchString(test.Name) {
				matchedTests = append(matchedTests, test)
				matchFound = true
			}
		}
		if len(matchedTests) == 0 {
			continue
		}

		suite.Tests = matchedTests
		dir, _ := filepath.Split(suite.Location)
		_ = os.MkdirAll(dir, os.ModePerm)
		err := os.WriteFile(suite.Location, []byte(suite.BashString()), os.ModePerm)
		if err != nil {
			return errors.Errorf("cannot save suite %v, : %v", suite.Name(), err.Error())
		}
	}

	if !matchFound {
		return errors.Errorf("No matches found for pattern: %s", matchRegex.String())
	}

	return nil
}

func getFilter(root string) func(string) bool {
	var ignored []string
	ignored = append(ignored, filepath.Join(root, ".git"))

	return func(s string) bool {
		for _, line := range ignored {
			if strings.HasPrefix(s, line) {
				return true
			}
		}
		return false
	}
}

func getRecursiveDirectories(root string) []string {
	var result []string
	var isIgnored = getFilter(root)
	_ = filepath.Walk(root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && !isIgnored(path) {
				result = append(result, path)
			}
			return nil
		})

	return result
}
