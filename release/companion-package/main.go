package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type target struct {
	GOOS   string
	GOARCH string
}

type packageOptions struct {
	Repository string
	Output     string
	Version    string
	Commit     string
	Epoch      time.Time
	Targets    []target
}

type provenance struct {
	Artifact     string `json:"artifact"`
	SHA256       string `json:"sha256"`
	Version      string `json:"version"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	SourceCommit string `json:"sourceCommit"`
}

type archiveFile struct {
	Name string
	Mode os.FileMode
	Body []byte
}

type targetFlags []string

func (values *targetFlags) String() string { return strings.Join(*values, ",") }
func (values *targetFlags) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	logger := slog.Default()
	var output, version, commit string
	var requested targetFlags
	flag.StringVar(&output, "output", "", "directory for release archives")
	flag.StringVar(&version, "version", "", "release version without a leading v")
	flag.StringVar(&commit, "commit", "", "source commit hash")
	flag.Var(&requested, "target", "GOOS/GOARCH target; repeat to limit the default matrix")
	flag.Parse()
	epoch, err := sourceDateEpoch()
	if err != nil {
		logger.Error("[release-package] invalid source date", "error_class", "source_date_epoch")
		os.Exit(2)
	}
	targets, err := parseTargets(requested)
	if err != nil {
		logger.Error("[release-package] invalid target", "error_class", "target")
		os.Exit(2)
	}
	repository, err := os.Getwd()
	if err != nil {
		logger.Error("[release-package] repository unavailable", "error_class", "working_directory")
		os.Exit(1)
	}
	options := packageOptions{Repository: repository, Output: output, Version: version, Commit: commit, Epoch: epoch, Targets: targets}
	logger.Info("[release-package] packaging requested", "input_shape", "version_commit_target_matrix", "target_count", len(targets))
	if err := packageCompanions(options); err != nil {
		logger.Error("[release-package] packaging failed", "error_class", "package")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logger.Info("[release-package] packaging completed", "output_shape", "archives_checksums", "archive_count", len(targets))
}

func packageCompanions(options packageOptions) error {
	if err := validateOptions(options); err != nil {
		return err
	}
	if err := os.MkdirAll(options.Output, 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "codex-launcher-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	checksums := map[string]string{}
	for _, platform := range options.Targets {
		archiveName, digest, err := packageTarget(options, platform, work)
		if err != nil {
			return fmt.Errorf("package %s/%s: %w", platform.GOOS, platform.GOARCH, err)
		}
		checksums[archiveName] = digest
	}
	notice, err := os.ReadFile(filepath.Join(options.Repository, "THIRD_PARTY_NOTICES.md"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(options.Output, "THIRD_PARTY_NOTICES.md"), notice, 0o644); err != nil {
		return err
	}
	noticeDigest := sha256.Sum256(notice)
	checksums["THIRD_PARTY_NOTICES.md"] = hex.EncodeToString(noticeDigest[:])
	return writeChecksums(filepath.Join(options.Output, "SHA256SUMS"), checksums)
}

func packageTarget(options packageOptions, platform target, work string) (string, string, error) {
	executable := "codex-launcher"
	if platform.GOOS == "windows" {
		executable += ".exe"
	}
	binaryPath := filepath.Join(work, platform.GOOS+"-"+platform.GOARCH+"-"+executable)
	command := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-buildid=", "-o", binaryPath, "./companion/cmd/codex-launcher")
	command.Dir = options.Repository
	command.Env = append(os.Environ(), "GOOS="+platform.GOOS, "GOARCH="+platform.GOARCH, "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("go build: %w: %s", err, strings.TrimSpace(string(output)))
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(binary)
	digestText := hex.EncodeToString(digest[:])
	metadata, err := json.Marshal(provenance{
		Artifact: executable, SHA256: digestText, Version: options.Version,
		GOOS: platform.GOOS, GOARCH: platform.GOARCH, SourceCommit: options.Commit,
	})
	if err != nil {
		return "", "", err
	}
	files := []archiveFile{
		{Name: executable, Mode: 0o755, Body: binary},
		{Name: executable + ".sha256", Mode: 0o644, Body: []byte(digestText + "  " + executable + "\n")},
		{Name: executable + ".provenance.json", Mode: 0o644, Body: append(metadata, '\n')},
	}
	for _, name := range []string{"LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md"} {
		body, err := os.ReadFile(filepath.Join(options.Repository, name))
		if err != nil {
			return "", "", err
		}
		files = append(files, archiveFile{Name: name, Mode: 0o644, Body: body})
	}
	archiveName := archiveFilename(options.Version, platform)
	archivePath := filepath.Join(options.Output, archiveName)
	if platform.GOOS == "windows" {
		err = writeZip(archivePath, files, options.Epoch)
	} else {
		err = writeTarGzip(archivePath, files, options.Epoch)
	}
	if err != nil {
		return "", "", err
	}
	archiveBody, err := os.ReadFile(archivePath)
	if err != nil {
		return "", "", err
	}
	archiveDigest := sha256.Sum256(archiveBody)
	return archiveName, hex.EncodeToString(archiveDigest[:]), nil
}

func writeTarGzip(path string, files []archiveFile, epoch time.Time) error {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	compressed, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		_ = output.Close()
		return err
	}
	compressed.Header.ModTime = epoch
	compressed.Header.OS = 255
	archive := tar.NewWriter(compressed)
	for _, file := range sortedArchiveFiles(files) {
		header := &tar.Header{Name: file.Name, Mode: int64(file.Mode.Perm()), Size: int64(len(file.Body)), ModTime: epoch, AccessTime: epoch, ChangeTime: epoch, Typeflag: tar.TypeReg}
		if err := archive.WriteHeader(header); err != nil {
			return closeArchive(output, archive, compressed, err)
		}
		if _, err := archive.Write(file.Body); err != nil {
			return closeArchive(output, archive, compressed, err)
		}
	}
	return closeArchive(output, archive, compressed, nil)
}

func closeArchive(output *os.File, archive *tar.Writer, compressed *gzip.Writer, prior error) error {
	return errors.Join(prior, archive.Close(), compressed.Close(), output.Sync(), output.Close())
}

func writeZip(path string, files []archiveFile, epoch time.Time) error {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(output)
	var writeErr error
	for _, file := range sortedArchiveFiles(files) {
		header := &zip.FileHeader{Name: file.Name, Method: zip.Deflate}
		header.SetMode(file.Mode)
		header.SetModTime(epoch)
		writer, err := archive.CreateHeader(header)
		if err == nil {
			_, err = writer.Write(file.Body)
		}
		if err != nil {
			writeErr = err
			break
		}
	}
	return errors.Join(writeErr, archive.Close(), output.Sync(), output.Close())
}

func sortedArchiveFiles(files []archiveFile) []archiveFile {
	result := append([]archiveFile(nil), files...)
	sort.Slice(result, func(left, right int) bool { return result[left].Name < result[right].Name })
	return result
}

func writeChecksums(path string, checksums map[string]string) error {
	names := make([]string, 0, len(checksums))
	for name := range checksums {
		names = append(names, name)
	}
	sort.Strings(names)
	var body strings.Builder
	for _, name := range names {
		fmt.Fprintf(&body, "%s  %s\n", checksums[name], name)
	}
	return os.WriteFile(path, []byte(body.String()), 0o644)
}

func validateOptions(options packageOptions) error {
	if options.Repository == "" || options.Output == "" || !safeToken(options.Version) || !validCommit(options.Commit) || options.Epoch.IsZero() || len(options.Targets) == 0 {
		return errors.New("output, version, commit, source date, and at least one target are required")
	}
	for _, platform := range options.Targets {
		if !validTarget(platform) {
			return fmt.Errorf("unsupported target %s/%s", platform.GOOS, platform.GOARCH)
		}
	}
	return nil
}

func parseTargets(values []string) ([]target, error) {
	if len(values) == 0 {
		return []target{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}}, nil
	}
	result := make([]target, 0, len(values))
	seen := map[target]bool{}
	for _, value := range values {
		parts := strings.Split(value, "/")
		if len(parts) != 2 {
			return nil, errors.New("target must use GOOS/GOARCH")
		}
		platform := target{GOOS: parts[0], GOARCH: parts[1]}
		if !validTarget(platform) || seen[platform] {
			return nil, errors.New("target is unsupported or duplicated")
		}
		seen[platform] = true
		result = append(result, platform)
	}
	return result, nil
}

func validTarget(platform target) bool {
	return (platform.GOOS == "darwin" || platform.GOOS == "linux" || platform.GOOS == "windows") && (platform.GOARCH == "amd64" || platform.GOARCH == "arm64")
}

func archiveFilename(version string, platform target) string {
	extension := ".tar.gz"
	if platform.GOOS == "windows" {
		extension = ".zip"
	}
	return "codex-launcher_" + version + "_" + platform.GOOS + "_" + platform.GOARCH + extension
}

func sourceDateEpoch() (time.Time, error) {
	value := os.Getenv("SOURCE_DATE_EPOCH")
	if value == "" {
		return time.Time{}, errors.New("SOURCE_DATE_EPOCH is required")
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}, errors.New("SOURCE_DATE_EPOCH must be a positive Unix timestamp")
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func safeToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '.' && character != '-' {
			return false
		}
	}
	return true
}

func validCommit(value string) bool {
	if len(value) < 7 || len(value) > 64 || len(value)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
