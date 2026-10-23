// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cliCase is one command run recorded from the reference command line (Node.js and Handlebars 4.7.10).
type cliCase struct {
	Argv   []string          `json:"argv"`
	Files  map[string]string `json:"files"`
	Stdout string            `json:"stdout"`
	Stderr string            `json:"stderr"`
	Status int               `json:"status"`
}

func TestRecordedRuns(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "cases.golden"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []cliCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	for i, c := range cases {
		dir := t.TempDir()
		for name, content := range c.Files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		status := Run(c.Argv, strings.NewReader(""), &stdout, &stderr)
		if stdout.String() != c.Stdout || stderr.String() != c.Stderr || status != c.Status {
			t.Errorf("case %d %q (template %q):\n got  %q %q %d\n want %q %q %d", i, c.Argv, c.Files["t.hbs"],
				stdout.String(), stderr.String(), status, c.Stdout, c.Stderr, c.Status)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "handlebars: expected TEMPLATE [DATA]\n"},
		{[]string{"--bogus", "t"}, "handlebars: unknown option --bogus\n"},
		{[]string{"t", "--partial"}, "handlebars: --partial needs a value\n"},
		{[]string{"--partial", "=x", "t"}, "handlebars: --partial needs NAME=VALUE\n"},
		{[]string{"missing-file.hbs"}, "handlebars: cannot read missing-file.hbs\n"},
		{[]string{"-", "-", "-"}, "handlebars: expected TEMPLATE [DATA]\n"},
	} {
		var stdout, stderr bytes.Buffer
		if status := Run(tc.args, strings.NewReader(""), &stdout, &stderr); status != 2 || stderr.String() != tc.want {
			t.Errorf("%q: status %d, stderr %q; want 2, %q", tc.args, status, stderr.String(), tc.want)
		}
	}
}
