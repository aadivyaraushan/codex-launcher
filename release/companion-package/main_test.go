package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestPackageOnePlatformIsReproducible(t *testing.T) {
	repository := repositoryRoot(t)
	epoch := time.Unix(1_700_000_000, 0).UTC()
	platform := target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	options := packageOptions{Repository: repository, Version: "0.1.0-alpha.1", Commit: "0123456789abcdef", Epoch: epoch, Targets: []target{platform}}

	options.Output = first
	if err := packageCompanions(options); err != nil {
		t.Fatal(err)
	}
	options.Output = second
	if err := packageCompanions(options); err != nil {
		t.Fatal(err)
	}

	firstFiles := directoryDigests(t, first)
	secondFiles := directoryDigests(t, second)
	if !reflect.DeepEqual(firstFiles, secondFiles) {
		t.Fatalf("release bytes changed between builds:\nfirst=%#v\nsecond=%#v", firstFiles, secondFiles)
	}
	archiveName := archiveFilename(options.Version, platform)
	archivePath := filepath.Join(first, archiveName)
	entries := archiveEntries(t, archivePath)
	wantExecutable := "codex-launcher"
	if runtime.GOOS == "windows" {
		wantExecutable += ".exe"
	}
	for _, required := range []string{wantExecutable, wantExecutable + ".sha256", wantExecutable + ".provenance.json", "LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md"} {
		if !contains(entries, required) {
			t.Fatalf("archive entries %v missing %q", entries, required)
		}
	}
	checksums, err := os.ReadFile(filepath.Join(first, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(checksums), "  "+archiveName+"\n") {
		t.Fatalf("SHA256SUMS does not name archive:\n%s", checksums)
	}
	contents := archiveContents(t, archivePath)
	assertNamedChecksum(t, checksums, archiveName, mustReadFile(t, archivePath))
	assertNamedChecksum(t, checksums, "THIRD_PARTY_NOTICES.md", mustReadFile(t, filepath.Join(first, "THIRD_PARTY_NOTICES.md")))
	assertEmbeddedMetadata(t, contents, wantExecutable, options, platform)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(directory, "..", ".."))
}

func directoryDigests(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		result[entry.Name()] = hex.EncodeToString(digest[:])
	}
	return result
}

func archiveEntries(t *testing.T, path string) []string {
	t.Helper()
	contents := archiveContents(t, path)
	names := make([]string, 0, len(contents))
	for name := range contents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func archiveContents(t *testing.T, path string) map[string][]byte {
	t.Helper()
	contents := map[string][]byte{}
	if strings.HasSuffix(path, ".zip") {
		reader, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		for _, file := range reader.File {
			entry, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(entry)
			closeErr := entry.Close()
			if readErr != nil || closeErr != nil {
				t.Fatal(errors.Join(readErr, closeErr))
			}
			contents[file.Name] = body
		}
	} else {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		compressed, err := gzip.NewReader(file)
		if err != nil {
			t.Fatal(err)
		}
		defer compressed.Close()
		reader := tar.NewReader(compressed)
		for {
			header, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			contents[header.Name] = body
		}
	}
	return contents
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func assertNamedChecksum(t *testing.T, checksums []byte, name string, body []byte) {
	t.Helper()
	wanted := sha256.Sum256(body)
	wantedText := hex.EncodeToString(wanted[:])
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			if fields[0] != wantedText {
				t.Fatalf("checksum for %s = %s, want %s", name, fields[0], wantedText)
			}
			return
		}
	}
	t.Fatalf("checksum for %s is missing", name)
}

func assertEmbeddedMetadata(t *testing.T, contents map[string][]byte, executable string, options packageOptions, platform target) {
	t.Helper()
	binary, ok := contents[executable]
	if !ok {
		t.Fatalf("archive is missing %s", executable)
	}
	digest := sha256.Sum256(binary)
	digestText := hex.EncodeToString(digest[:])
	if got := string(contents[executable+".sha256"]); got != digestText+"  "+executable+"\n" {
		t.Fatalf("embedded checksum = %q", got)
	}
	var metadata provenance
	if err := json.Unmarshal(contents[executable+".provenance.json"], &metadata); err != nil {
		t.Fatal(err)
	}
	wanted := provenance{
		Artifact: executable, SHA256: digestText, Version: options.Version,
		GOOS: platform.GOOS, GOARCH: platform.GOARCH, SourceCommit: options.Commit,
	}
	if !reflect.DeepEqual(metadata, wanted) {
		t.Fatalf("embedded provenance = %#v, want %#v", metadata, wanted)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
