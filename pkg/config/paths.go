package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ptone/scion-agent/pkg/util"
)

const (
	DotScion = ".scion"
	GlobalDir = ".scion"
)

// GetRepoDir returns the .scion directory at the root of the git repo, if it exists.
func GetRepoDir() (string, bool) {
	if !util.IsGitRepo() {
		return "", false
	}
	root, err := util.RepoRoot()
	if err != nil {
		return "", false
	}
	p := filepath.Join(root, DotScion)
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p, true
	}
	return "", false
}

// FindProjectRoot walks up the directory tree to find the .scion directory.
func FindProjectRoot() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}

	dir := wd
	for {
		p := filepath.Join(dir, DotScion)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p, true
		}

		parent := filepath.Dir(dir)
		if parent == dir { // Reached root
			break
		}
		dir = parent
	}
	return "", false
}

// GetResolvedProjectDir returns the active .scion directory based on precedence.
func GetResolvedProjectDir(explicitPath string) (string, error) {
	// 1. Explicitly provided via flag
	if explicitPath != "" {
		if explicitPath == "home" || explicitPath == "global" {
			return GetGlobalDir()
		}
		abs, err := filepath.Abs(explicitPath)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	// 2. Walk up to find .scion (covers repo root and current dir cases)
	if p, ok := FindProjectRoot(); ok {
		return p, nil
	}

	// 3. Fallback to global
	return GetGlobalDir()
}

func GetProjectDir() (string, error) {
	// 1. Walk up to find .scion
	if p, ok := FindProjectRoot(); ok {
		return p, nil
	}

	// 2. Fallback to current directory (legacy/non-repo behavior)
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, DotScion), nil
}

// GetGroveName returns the slugified name of the grove.
func GetGroveName(projectDir string) string {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return "unknown"
	}

	parent := filepath.Dir(abs)
	home, err := os.UserHomeDir()
	if err == nil && parent == home {
		return "global"
	}

	return slugify(filepath.Base(parent))
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var res strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			res.WriteRune(r)
		} else {
			res.WriteRune('-')
		}
	}
	return strings.Trim(res.String(), "-")
}

// GetTargetProjectDir returns the directory where a grove should be initialized.
func GetTargetProjectDir() (string, error) {
	// 1. Root of the current git repo if run inside a repo
	if util.IsGitRepo() {
		root, err := util.RepoRoot()
		if err == nil {
			return filepath.Join(root, DotScion), nil
		}
	}

	// 2. Current directory
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, DotScion), nil
}

func GetGlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, GlobalDir), nil
}

func GetProjectTemplatesDir() (string, error) {
	p, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "templates"), nil
}

func GetGlobalTemplatesDir() (string, error) {
	g, err := GetGlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(g, "templates"), nil
}

func GetProjectAgentsDir() (string, error) {
	p, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "agents"), nil
}

func GetProjectKubernetesConfigPath() (string, error) {
	p, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "kubernetes-config.json"), nil
}

func GetGlobalAgentsDir() (string, error) {
	g, err := GetGlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(g, "agents"), nil
}
