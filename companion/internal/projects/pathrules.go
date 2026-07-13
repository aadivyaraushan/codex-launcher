package projects

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

var projectIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validConfig(config Config) bool {
	return projectIDPattern.MatchString(config.ID) && validDisplayName(config.DisplayName) && filepath.IsAbs(config.Path)
}

func validDisplayName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 128 && strings.IndexFunc(name, unicode.IsControl) < 0
}

func inspectProjectPath(path string) (string, os.FileInfo, error) {
	cleaned := filepath.Clean(path)
	if runtime.GOOS == "windows" && strings.HasPrefix(cleaned, `\\`) {
		return "", nil, ErrUnsafeProjectPath
	}
	if err := rejectRedirectedComponents(cleaned); err != nil {
		return "", nil, err
	}
	canonical, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", nil, err
	}
	if !info.IsDir() {
		return "", nil, ErrUnsafeProjectPath
	}
	return canonical, info, nil
}

func rejectRedirectedComponents(path string) error {
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	current := volume + string(filepath.Separator)
	for _, component := range strings.Split(strings.Trim(remainder, `/\`), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode().Type() != os.ModeDir {
			return ErrUnsafeProjectPath
		}
	}
	return nil
}
