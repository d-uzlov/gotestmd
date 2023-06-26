// Copyright (c) 2020-2022 Doc.ai and/or its affiliates.
//
// Copyright (c) 2022 Cisco and/or its affiliates.
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

package generator

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

const suiteTemplate = `// Code generated by gotestmd DO NOT EDIT.
package {{ .Name }}

import(
	{{ .Imports }}
)

type Suite struct {
	{{ .Fields }}
}

func (s *Suite) SetupSuite() {
	{{ .Setup }}
	{{ if or .Run .Cleanup }}
	r := s.Runner("{{.Dir}}")
	{{ end }}
	{{ .Cleanup }}
	{{ .Run }}

{{ if .TestIncludedSuites }}
	s.RunIncludedSuites()
}

func (s *Suite) RunIncludedSuites() {
	{{ .TestIncludedSuites }}
{{ end }}
}
`

const includedSuiteTemplate = `
	{{ range .Suites }}
		s.Run("{{ .Title }}", func() {
			suite.Run(s.T(), &s.{{ .Name }}Suite)
		})
	{{ end }}
`

// Body represents a body of the method
type Body []string

// String returns the body as part of the method
func (b Body) String() string {
	var sb strings.Builder

	if len(b) == 0 {
		return ""
	}

	for _, block := range b {
		sb.WriteString("r.Run(")
		var lines = strings.Split(block, "\n")
		for i, line := range lines {
			sb.WriteString("`")
			sb.WriteString(line)
			sb.WriteString("`")
			if i+1 < len(lines) {
				sb.WriteString("+\"\\n\"+")
			}
		}
		sb.WriteString(")\n")
	}

	return sb.String()
}

// Suite represents a template for generating a testify suite.Suite
type Suite struct {
	Dir      string
	Location string
	Dependency
	Cleanup     Body
	Run         Body
	Tests       []*Test
	Children    []*Suite
	Parents     []*Suite
	Deps        Dependencies
	DepsToSetup Dependencies
}

func (s *Suite) generateChildrenTesting() string {
	tmpl, err := template.New("test").Parse(includedSuiteTemplate)
	if err != nil {
		panic(err.Error())
	}

	type suiteData struct {
		Title string
		Name  string
	}

	if len(s.Children) == 0 {
		return ""
	}

	var suites []*suiteData
	for _, child := range s.Children {
		_, title := path.Split(child.Dir)
		title = strings.Title(nameRegex.ReplaceAllString(title, "_"))
		suite := &suiteData{
			Title: title,
			Name:  child.Name(),
		}

		suites = append(suites, suite)
	}

	var result = new(strings.Builder)
	err = tmpl.Execute(result, struct {
		Suites []*suiteData
	}{
		Suites: suites,
	})
	if err != nil {
		panic(err.Error())
	}
	return result.String()
}

// String returns a string that contains generated testify.Suite
func (s *Suite) String() string {
	tmpl, err := template.New("test").Parse(
		suiteTemplate,
	)

	if err != nil {
		panic(err.Error())
	}

	cleanup := s.Cleanup.String()
	if len(cleanup) > 0 {
		cleanup = fmt.Sprintf(`	s.T().Cleanup(func() {
		%v
	})`, cleanup)
	}

	var result = new(strings.Builder)

	_ = tmpl.Execute(result, struct {
		Dir                string
		Name               string
		Cleanup            string
		Run                string
		Fields             string
		Imports            string
		Setup              string
		TestIncludedSuites string
	}{
		Dir:                s.Dir,
		Name:               s.Name(),
		Cleanup:            cleanup,
		Run:                s.Run.String(),
		Imports:            s.Deps.String(),
		Fields:             s.Deps.FieldsString(),
		Setup:              s.DepsToSetup.SetupString(),
		TestIncludedSuites: s.generateChildrenTesting(),
	})

	if len(s.Tests) == 0 {
		s.Tests = append(s.Tests, new(Test))
	}

	for _, test := range s.Tests {
		_, _ = result.WriteString(test.String())
	}

	return spaceRegex.ReplaceAllString(strings.TrimSpace(result.String()), "\n")
}

const bashSuiteTemplate = `
function setup() {
	{{ .Setup }}
}

function cleanup() {
	{{ .Cleanup }}
}
setup()
cleanup()
`

func (s *Suite) BashString() string {
	setup := strings.Join(s.getCompleteSetup(), "\n")
	cleanup := strings.Join(s.getCompleteCleanup(), "\n")

	tmpl, err := template.New("test").Parse(
		bashSuiteTemplate,
	)

	if err != nil {
		panic(err.Error())
	}

	var result = new(strings.Builder)

	_ = tmpl.Execute(result, struct {
		Dir     string
		Cleanup string
		Setup   string
	}{
		Dir:     s.Dir,
		Cleanup: cleanup,
		Setup:   setup,
	})

	return spaceRegex.ReplaceAllString(strings.TrimSpace(result.String()), "\n")
}

func (s *Suite) getCompleteSetup() []string {
	setup := make([]string, 0)
	for _, p := range s.Parents {
		setup = append(setup, p.getCompleteSetup()...)
	}

	absDir, _ := filepath.Abs(s.Dir)
	setup = append(setup, "cd "+absDir)
	setup = append(setup, s.Run...)
	return setup
}

func (s *Suite) getCompleteCleanup() []string {
	cleanup := s.Cleanup
	for _, p := range s.Parents {
		cleanup = append(cleanup, p.getCompleteCleanup()...)
	}

	return cleanup
}
