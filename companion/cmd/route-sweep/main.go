// Command route-sweep drives a corpus of raw utterances through the real
// phone router end to end — stage 1's explicit-app model, then stage 2's
// resolver — and reports where the actual route diverges from what the
// corpus expects. Run from cmd/route-sweep, or pass --corpus explicitly.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/discovery"
)

func main() {
	corpusPath := flag.String("corpus", "testdata/corpus.jsonl", "path to a JSONL corpus of discovery.Case")
	outPath := flag.String("out", "", "TSV output path (default stdout)")
	flag.Parse()

	if err := run(*corpusPath, *outPath, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "route-sweep:", err)
		os.Exit(1)
	}
}

func run(corpusPath, outPath string, stdout, stderr io.Writer) error {
	cases, err := loadCorpus(corpusPath)
	if err != nil {
		return err
	}
	sweep, err := discovery.NewPhoneSweep(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return fmt.Errorf("build phone sweep: %w", err)
	}
	results, err := sweep.Run(context.Background(), cases)
	if err != nil {
		return fmt.Errorf("run sweep: %w", err)
	}

	out := stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", outPath, err)
		}
		defer f.Close()
		out = f
	}
	writeTSV(out, results)
	writeSummary(stderr, results)
	return nil
}

func loadCorpus(path string) ([]discovery.Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cases []discovery.Case
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var c discovery.Case
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
		cases = append(cases, c)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cases, nil
}

var tsvColumns = []string{
	"utterance", "beeper", "actual_app", "actual_class", "actual_verb", "actual_operation",
	"decision", "question", "expected_app", "expected_class", "expected_verb", "verdict",
	"route_ok", "fail_kind", "reason",
}

func writeTSV(w io.Writer, results []discovery.Result) {
	fmt.Fprintln(w, strings.Join(tsvColumns, "\t"))
	for _, r := range results {
		row := []string{
			r.Utterance, r.Beeper, r.ActualApp, r.ActualClass, r.ActualVerb, r.ActualOperation,
			r.Decision, r.Question, r.ExpectedApp, r.ExpectedClass, r.ExpectedVerb, string(r.Verdict),
			strconv.FormatBool(r.RouteOK), r.FailKind, r.Reason,
		}
		for i, field := range row {
			row[i] = tsvSafe(field)
		}
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
}

func tsvSafe(field string) string {
	field = strings.ReplaceAll(field, "\t", " ")
	field = strings.ReplaceAll(field, "\n", " ")
	return field
}

func writeSummary(w io.Writer, results []discovery.Result) {
	counts := map[discovery.Verdict]int{}
	failKindCounts := map[string]int{}
	routeOKCount := 0
	var misroutes, deadEnds []discovery.Result
	for _, r := range results {
		counts[r.Verdict]++
		if r.RouteOK {
			routeOKCount++
		}
		if r.FailKind != "" {
			failKindCounts[r.FailKind]++
		}
		switch r.Verdict {
		case discovery.MISROUTE:
			misroutes = append(misroutes, r)
		case discovery.DEAD_END:
			deadEnds = append(deadEnds, r)
		}
	}

	fmt.Fprintf(w, "route-sweep: %d cases run\n", len(results))
	for _, v := range []discovery.Verdict{discovery.PASS, discovery.MISROUTE, discovery.DEAD_END, discovery.CAPABILITY_GAP} {
		fmt.Fprintf(w, "  %s: %d\n", v, counts[v])
	}

	fmt.Fprintln(w, "FailKind breakdown:")
	kinds := make([]string, 0, len(failKindCounts))
	for kind := range failKindCounts {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		fmt.Fprintf(w, "  %s: %d\n", kind, failKindCounts[kind])
	}
	fmt.Fprintf(w, "Routing: %d/%d RouteOK\n", routeOKCount, len(results))

	if len(misroutes) > 0 {
		fmt.Fprintln(w, "MISROUTE:")
		for _, r := range misroutes {
			fmt.Fprintf(w, "  [beeper=%s] %q -> app=%q class=%q verb=%q (expected app=%q class=%q verb=%q): %s\n",
				r.Beeper, r.Utterance, r.ActualApp, r.ActualClass, r.ActualVerb, r.ExpectedApp, r.ExpectedClass, r.ExpectedVerb, r.Reason)
		}
	}
	if len(deadEnds) > 0 {
		fmt.Fprintln(w, "DEAD_END:")
		for _, r := range deadEnds {
			fmt.Fprintf(w, "  [beeper=%s] %q -> %s\n", r.Beeper, r.Utterance, r.Question)
		}
	}
}
