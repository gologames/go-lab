package main

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCSVNoSpaceUnique(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		csv     string
		want    []string
		wantErr error
	}{
		{"single", "a", []string{"a"}, nil},
		{"multiple", "a,b,c", []string{"a", "b", "c"}, nil},
		{"empty token", "a,,c", nil, ErrInvalidCSV},
		{"whitespace token", "a, b,c", nil, ErrInvalidCSV},
		{"duplicate", "a,a", nil, ErrDuplicateCSV},
		{"duplicate three", "x,y,x", nil, ErrDuplicateCSV},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseCSVNoSpaceUnique(testCase.csv)
			if testCase.wantErr != nil {
				require.ErrorIs(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.want, got)
		})
	}
}

func TestSuffixRegex(t *testing.T) {
	t.Parallel()
	re, err := suffixRegex("connector")
	require.NoError(t, err)
	require.True(t, re.MatchString("services/connector"), "expected match for services/connector")
	require.False(t, re.MatchString("connectorx"), "expected no match for connectorx")

	_, err = suffixRegex("[")
	require.Error(t, err)
}

func TestWithGoWork(t *testing.T) {
	t.Parallel()
	got := withGoWork(nil, "/path/to/go.work")
	require.Equal(t, []string{"GOWORK=/path/to/go.work"}, got)

	got = withGoWork([]string{"PATH=/bin", "HOME=/home"}, "/a/go.work")
	require.Equal(t, []string{"PATH=/bin", "HOME=/home", "GOWORK=/a/go.work"}, got)

	got = withGoWork([]string{"GOWORK=old", "PATH=/bin"}, "/new/go.work")
	require.Equal(t, []string{"PATH=/bin", "GOWORK=/new/go.work"}, got)
}

func TestParseOverrides(t *testing.T) {
	t.Parallel()
	got, err := parseOverrides("")
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = parseOverrides("api:go build ./...")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "go build ./...", got["api"].cmd)

	got, err = parseOverrides("api,connector:golangci-lint run")
	require.NoError(t, err)
	require.Len(t, got, 2)

	_, err = parseOverrides("no-colon")
	require.ErrorIs(t, err, ErrOverrideFormat)

	_, err = parseOverrides("api:cmd \"quoted\"")
	require.ErrorIs(t, err, ErrOverrideQuotes)
}

func TestFilterModules(t *testing.T) {
	t.Parallel()
	modules := []string{
		filepath.Join("libs", "pkg", "uuid"),
		filepath.Join("libs", "pkg", "esrc"),
		filepath.Join("services", "api"),
		filepath.Join("services", "connector"),
	}

	got, err := filterModules(modules, "", "")
	require.NoError(t, err)
	require.Len(t, got, 4)

	got, err = filterModules(modules, "connector", "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "services/connector", filepath.ToSlash(got[0]))

	got, err = filterModules(modules, "", "api")
	require.NoError(t, err)
	require.Len(t, got, 3)

	got, err = filterModules(modules, "pkg/.*", "")
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestGetArgs(t *testing.T) {
	t.Parallel()
	defaultArgs := []string{"go", "build", "./..."}

	got, err := getArgs("services/api", defaultArgs, nil)
	require.NoError(t, err)
	require.Equal(t, defaultArgs, got)

	overrides, err := parseOverrides("api:golangci-lint run")
	require.NoError(t, err)
	got, err = getArgs("services/api", defaultArgs, overrides)
	require.NoError(t, err)
	require.Equal(t, []string{"golangci-lint", "run"}, got)

	got, err = getArgs("services/connector", defaultArgs, overrides)
	require.NoError(t, err)
	require.Equal(t, defaultArgs, got)

	overrides, _ = parseOverrides("api:cmd1")
	ov2, _ := parseOverrides("services/api:cmd2")
	maps.Copy(overrides, ov2)
	got, err = getArgs("services/api", defaultArgs, overrides)
	require.NoError(t, err)
	require.Equal(t, []string{"cmd2"}, got)
}
