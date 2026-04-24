// Command workspace runs a given command in each module of the Go workspace (go.work).
// It supports parallel execution, filtering by module, and per-module command overrides.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/samber/lo"
	"github.com/samber/oops"
	"golang.org/x/sync/errgroup"
)

const (
	envGOWORK               = "GOWORK="
	workspaceModulesTimeout = 10 * time.Second
)

var (
	ErrWorkfileNotGoWork    = errors.New("-workfile must point to go.work")
	ErrWorkfileIsDir        = errors.New("-workfile points to a directory")
	ErrOverrideAmbiguous    = errors.New("override ambiguous: same-length matches")
	ErrGoWorkNotFoundInRepo = errors.New("go.work not found from cwd up to git repo root")
	ErrGoWorkNotFoundInRoot = errors.New("go.work not found from cwd up to filesystem root")
	ErrInvalidCSV           = errors.New("invalid csv (empty or contains whitespace)")
	ErrDuplicateCSV         = errors.New("duplicate values in csv")
	ErrOverrideFormat       = errors.New("override: expected module1,module2,...:command")
	ErrOverrideQuotes       = errors.New("override: quotes are not supported in command")
	ErrPanic                = errors.New("panic in module")
)

func main() {
	workfile := flag.String("workfile", "", "Path to go.work file (default: find in current or parent dirs)")
	parallel := flag.Int("parallel", 0, "Max concurrent modules (0=auto: min(NumCPU, modules), 1=sequential, 2+=N-way). Must be >= 0")
	only := flag.String("only", "", "Comma-separated suffix regexes to include (e.g. service,common or libs/.*; matched from end of module path)")
	exclude := flag.String("exclude", "", "Comma-separated suffix regexes to exclude (matched from end of module path)")
	overrideStr := flag.String("override", "", "Override command for modules: module1,module2,...:command (e.g. service,common:golangci-lint run)")
	flag.Parse()

	workPath, err := resolveWorkPath(*workfile)
	if err != nil {
		log.Fatalf("workspace: %v", err)
	}

	ctx := context.Background()
	modules, err := workspaceModules(ctx, workPath)
	if err != nil {
		log.Fatalf("workspace: %v", err)
	}

	modules, err = filterModules(modules, *only, *exclude)
	if err != nil {
		log.Fatalf("workspace: %v", err)
	}
	if len(modules) == 0 {
		log.Fatal("workspace: no modules left after -only/-exclude")
	}

	defaultArgs := flag.Args()
	if len(defaultArgs) == 0 {
		lo.ForEach(modules, func(m string, _ int) { log.Println(m) })
		return
	}

	overrides, err := parseOverrides(*overrideStr)
	if err != nil {
		log.Fatalf("workspace: %v", err)
	}
	if *parallel < 0 {
		log.Fatalf("workspace: parallel must be >= 0, got %d", *parallel)
	}

	if err := runModules(ctx, workPath, modules, defaultArgs, overrides, *parallel); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func resolveWorkPath(workfile string) (string, error) {
	if workfile == "" {
		return findGoWork()
	}
	if filepath.Base(workfile) != "go.work" {
		return "", ErrWorkfileNotGoWork
	}

	abs, err := filepath.Abs(workfile)
	if err != nil {
		return "", fmt.Errorf("abs workfile: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat workfile: %w", err)
	}

	if info.IsDir() {
		return "", ErrWorkfileIsDir
	}
	return abs, nil
}

func findGoWork() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", oops.Wrap(err)
	}

	for {
		path := filepath.Join(dir, "go.work")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", oops.Wrap(err)
		}

		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return "", fmt.Errorf("%w: %s", ErrGoWorkNotFoundInRepo, dir)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", oops.Wrap(err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w: %s", ErrGoWorkNotFoundInRoot, dir)
		}
		dir = parent
	}
}

// goWorkEdit is the JSON shape of "go work edit -json".
type goWorkEdit struct {
	Use []useEntry `json:"Use"` //nolint:tagliatelle
}

type useEntry struct {
	DiskPath string `json:"DiskPath"` //nolint:tagliatelle
}

// workspaceModules returns module dirs from "go work edit -json" (GOWORK=workPath).
func workspaceModules(ctx context.Context, workPath string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, workspaceModulesTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "work", "edit", "-json")

	workDir := filepath.Dir(workPath)
	cmd.Dir = workDir
	cmd.Env = withGoWork(os.Environ(), workPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if ctx.Err() != nil {
			if msg == "" {
				return nil, oops.Wrap(ctx.Err())
			}
			return nil, fmt.Errorf("%w: %s", ctx.Err(), msg)
		}
		return nil, fmt.Errorf("go work edit -json: %w: %s", err, msg)
	}

	var edit goWorkEdit
	if err := json.Unmarshal(out, &edit); err != nil {
		return nil, fmt.Errorf("parse go.work json: %w: %s", err, strings.TrimSpace(string(out)))
	}

	modules := lo.Map(edit.Use, func(u useEntry, _ int) string {
		module := strings.TrimSpace(u.DiskPath)
		if !filepath.IsAbs(module) {
			module = filepath.Join(workDir, module)
		}
		return filepath.Clean(module)
	})

	return modules, nil
}

func withGoWork(env []string, workPath string) []string {
	env = lo.Reject(env, func(e string, _ int) bool { return strings.HasPrefix(e, envGOWORK) })
	return append(env, envGOWORK+workPath)
}

func suffixRegex(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern + "$")
	if err != nil {
		return nil, oops.Wrap(err)
	}
	return re, nil
}

func parseCSVNoSpaceUnique(csv string) ([]string, error) {
	tokens := strings.Split(csv, ",")
	if lo.SomeBy(tokens, func(s string) bool { return s == "" || strings.IndexFunc(s, unicode.IsSpace) >= 0 }) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCSV, csv)
	}

	if len(lo.Uniq(tokens)) != len(tokens) {
		return nil, fmt.Errorf("%w: %q", ErrDuplicateCSV, csv)
	}
	return tokens, nil
}

// filterModules applies -only/-exclude suffix regexes to slash-normalized module paths.
func filterModules(modules []string, onlyStr, excludeStr string) ([]string, error) {
	parseSuffixREs := func(csv string) ([]*regexp.Regexp, error) {
		if csv == "" {
			return nil, nil
		}

		tokens, err := parseCSVNoSpaceUnique(csv)
		if err != nil {
			return nil, err
		}

		var res []*regexp.Regexp
		for _, pattern := range tokens {
			patternRe, err := suffixRegex(pattern)
			if err != nil {
				return nil, fmt.Errorf("pattern %q: %w", pattern, err)
			}
			res = append(res, patternRe)
		}
		return res, nil
	}

	onlyRE, err := parseSuffixREs(onlyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid -only: %w", err)
	}
	excludeRE, err := parseSuffixREs(excludeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid -exclude: %w", err)
	}

	pathMatches := func(p string, res []*regexp.Regexp) bool {
		return lo.SomeBy(res, func(r *regexp.Regexp) bool { return r.MatchString(p) })
	}
	out := lo.Filter(modules, func(module string, _ int) bool {
		normalizedPath := filepath.ToSlash(module)
		return !pathMatches(normalizedPath, excludeRE) &&
			(len(onlyRE) == 0 || pathMatches(normalizedPath, onlyRE))
	})
	return out, nil
}

type override struct {
	re  *regexp.Regexp
	cmd string
}

// parseOverrides parses override in form "module1,module2,...,moduleN:command".
func parseOverrides(overrideStr string) (map[string]override, error) {
	if overrideStr == "" {
		return map[string]override{}, nil
	}

	modulesStr, cmd, ok := strings.Cut(overrideStr, ":")
	if !ok || modulesStr == "" || strings.TrimSpace(cmd) == "" || strings.Contains(cmd, ":") {
		return nil, fmt.Errorf("%w: %q", ErrOverrideFormat, overrideStr)
	}
	if strings.ContainsAny(cmd, `"'`) {
		return nil, fmt.Errorf("%w: %q", ErrOverrideQuotes, overrideStr)
	}

	modules, err := parseCSVNoSpaceUnique(modulesStr)
	if err != nil {
		return nil, fmt.Errorf("override %q: %w", overrideStr, err)
	}

	out := make(map[string]override, len(modules))
	for _, module := range modules {
		moduleRe, err := suffixRegex(module)
		if err != nil {
			return nil, err
		}
		out[module] = override{re: moduleRe, cmd: cmd}
	}
	return out, nil
}

func runModules(ctx context.Context, workPath string, modules []string, defaultArgs []string, overrides map[string]override, parallel int) error {
	parallel = lo.Ternary(parallel != 0, parallel, min(runtime.NumCPU(), len(modules)))
	var outputMu sync.Mutex

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(parallel)

	for _, module := range modules {
		group.Go(func() (runErr error) {
			args, err := getArgs(module, defaultArgs, overrides)
			if err != nil {
				return fmt.Errorf("%s: %w", module, err)
			}

			var out []byte
			defer func() {
				flushOutput(&outputMu, module, args, out)
				if r := recover(); r != nil {
					runErr = fmt.Errorf("%w: %s: %v\n%s", ErrPanic, module, r, debug.Stack())
				}
			}()

			out, runErr = runOne(ctx, workPath, module, args)
			return runErr
		})
	}

	return oops.Wrap(group.Wait())
}

// getArgs returns the command args matched by suffix-regex overrides (longest match wins), or defaultArgs.
func getArgs(module string, defaultArgs []string, overrides map[string]override) ([]string, error) {
	normalizedPath := filepath.ToSlash(module)
	var bestKey string
	ambiguous := false

	for key, override := range overrides {
		if !override.re.MatchString(normalizedPath) {
			continue
		}
		if len(key) > len(bestKey) {
			bestKey = key
			ambiguous = false
			continue
		}
		if len(key) == len(bestKey) {
			ambiguous = true
		}
	}

	if ambiguous {
		return nil, ErrOverrideAmbiguous
	}
	if bestKey == "" {
		return defaultArgs, nil
	}
	return strings.Fields(overrides[bestKey].cmd), nil
}

// runOne runs the command in the module dir (absolute) and returns combined output and any error.
func runOne(ctx context.Context, workPath string, moduleDir string, args []string) ([]byte, error) {
	command := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
	command.Dir = moduleDir
	command.Env = withGoWork(os.Environ(), workPath)

	out, err := command.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s: %q: %w", moduleDir, strings.Join(args, " "), err)
	}
	return out, nil
}

// flushOutput serializes output so parallel modules don't interleave.
func flushOutput(mu *sync.Mutex, module string, args []string, out []byte) {
	mu.Lock()
	defer mu.Unlock()
	log.Printf("[%s] %s\n%s", module, strings.Join(args, " "), out)
}
