// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	statusStart = "<!-- status autogenerated section -->"
	statusEnd   = "<!-- end autogenerated section -->"
)

func main() {
	flag.Parse()
	yml := flag.Arg(0)
	if err := run(yml); err != nil {
		log.Fatal(err)
	}
}

func run(ymlPath string) error {
	if ymlPath == "" {
		return errors.New("argument must be metadata.yaml file")
	}
	ymlPath, err := filepath.Abs(ymlPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %v: %w", ymlPath, err)
	}

	ymlDir := filepath.Dir(ymlPath)

	md, err := loadMetadata(ymlPath)
	if err != nil {
		return fmt.Errorf("failed loading %v: %w", ymlPath, err)
	}

	tmplDir := "templates"

	codeDir := filepath.Join(ymlDir, "internal", "metadata")
	if err = os.MkdirAll(codeDir, 0700); err != nil {
		return fmt.Errorf("unable to create output directory %q: %w", codeDir, err)
	}
	if len(md.Status.Stability) > 0 {
		if err = generateFile(filepath.Join(tmplDir, "status.go.tmpl"),
			filepath.Join(codeDir, "generated_status.go"), md); err != nil {
			return err
		}

		if err = inlineReplace(
			filepath.Join(tmplDir, "readme.md.tmpl"),
			filepath.Join(ymlDir, "README.md"),
			md, statusStart, statusEnd); err != nil {
			return err
		}
	}
	if len(md.Metrics) == 0 && len(md.ResourceAttributes) == 0 {
		return nil
	}

	if err = os.MkdirAll(filepath.Join(codeDir, "testdata"), 0700); err != nil {
		return fmt.Errorf("unable to create output directory %q: %w", filepath.Join(codeDir, "testdata"), err)
	}
	if err = generateFile(filepath.Join(tmplDir, "testdata", "config.yaml.tmpl"),
		filepath.Join(codeDir, "testdata", "config.yaml"), md); err != nil {
		return err
	}

	if err = generateFile(filepath.Join(tmplDir, "config.go.tmpl"),
		filepath.Join(codeDir, "generated_config.go"), md); err != nil {
		return err
	}
	if err = generateFile(filepath.Join(tmplDir, "config_test.go.tmpl"),
		filepath.Join(codeDir, "generated_config_test.go"), md); err != nil {
		return err
	}

	if len(md.Metrics) == 0 {
		return nil
	}

	if err = generateFile(filepath.Join(tmplDir, "metrics.go.tmpl"),
		filepath.Join(codeDir, "generated_metrics.go"), md); err != nil {
		return err
	}
	if err = generateFile(filepath.Join(tmplDir, "metrics_test.go.tmpl"),
		filepath.Join(codeDir, "generated_metrics_test.go"), md); err != nil {
		return err
	}

	return generateFile(filepath.Join(tmplDir, "documentation.md.tmpl"), filepath.Join(ymlDir, "documentation.md"), md)
}

func templatize(tmplFile string, md metadata) *template.Template {
	return template.Must(
		template.
			New(filepath.Base(tmplFile)).
			Option("missingkey=error").
			Funcs(map[string]interface{}{
				"publicVar": func(s string) (string, error) {
					return formatIdentifier(s, true)
				},
				"attributeInfo": func(an attributeName) attribute {
					return md.Attributes[an]
				},
				"attributeName": func(an attributeName) string {
					if md.Attributes[an].NameOverride != "" {
						return md.Attributes[an].NameOverride
					}
					return string(an)
				},
				"metricInfo": func(mn metricName) metric {
					return md.Metrics[mn]
				},
				"parseImportsRequired": func(metrics map[metricName]metric) bool {
					for _, m := range metrics {
						if m.Data().HasMetricInputType() {
							return true
						}
					}
					return false
				},
				"stringsJoin": strings.Join,
				"casesTitle":  cases.Title(language.English).String,
				"inc":         func(i int) int { return i + 1 },
				"distroURL": func(name string) string {
					return distros[name]
				},
				// ParseFS delegates the parsing of the files to `Glob`
				// which uses the `\` as a special character.
				// Meaning on windows based machines, the `\` needs to be replaced
				// with a `/` for it to find the file.
			}).ParseFS(templateFS, strings.ReplaceAll(tmplFile, "\\", "/")))
}

func inlineReplace(tmplFile string, outputFile string, md metadata, start string, end string) error {
	var readmeContents []byte
	var err error
	if readmeContents, err = os.ReadFile(outputFile); err != nil {
		return err
	}

	var re = regexp.MustCompile(fmt.Sprintf("%s[\\s\\S]*%s", start, end))
	if !re.Match(readmeContents) {
		return nil
	}

	tmpl := templatize(tmplFile, md)
	buf := bytes.Buffer{}

	if err := tmpl.Execute(&buf, templateContext{metadata: md, Package: "metadata"}); err != nil {
		return fmt.Errorf("failed executing template: %w", err)
	}

	result := buf.String()

	s := re.ReplaceAllString(string(readmeContents), result)
	if err := os.WriteFile(outputFile, []byte(s), 0600); err != nil {
		return fmt.Errorf("failed writing %q: %w", outputFile, err)
	}

	return nil
}

func generateFile(tmplFile string, outputFile string, md metadata) error {
	tmpl := templatize(tmplFile, md)
	buf := bytes.Buffer{}

	if err := tmpl.Execute(&buf, templateContext{metadata: md, Package: "metadata"}); err != nil {
		return fmt.Errorf("failed executing template: %w", err)
	}

	if err := os.Remove(outputFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("unable to remove genererated file %q: %w", outputFile, err)
	}

	result := buf.Bytes()
	var formatErr error
	if strings.HasSuffix(outputFile, ".go") {
		if formatted, err := format.Source(buf.Bytes()); err == nil {
			result = formatted
		} else {
			formatErr = fmt.Errorf("failed formatting %s:%w", outputFile, err)
		}
	}

	if err := os.WriteFile(outputFile, result, 0600); err != nil {
		return fmt.Errorf("failed writing %q: %w", outputFile, err)
	}

	return formatErr
}
