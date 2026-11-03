// Command testreport reads what go test -json wrote: it prints the output of what failed,
// which plain go test prints by itself, and what took the time, which it doesn't.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

// event is a line of go test -json.
type event struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
	Output  string
}

// result is how a package, or a test of one, went.
type result struct {
	pkg, test string
	elapsed   float64
	failed    bool
	output    strings.Builder
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: testreport test.json")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	results := map[[2]string]*result{}
	lines := bufio.NewScanner(f)
	lines.Buffer(nil, 64<<20)
	for lines.Scan() {
		var e event
		if json.Unmarshal(lines.Bytes(), &e) != nil {
			fmt.Println(lines.Text()) // what go itself said, as a build failing
			continue
		}
		key := [2]string{e.Package, e.Test}
		r := results[key]
		if r == nil {
			r = &result{pkg: e.Package, test: e.Test}
			results[key] = r
		}
		switch e.Action {
		case "output":
			r.output.WriteString(e.Output)
		case "pass", "fail", "skip":
			r.elapsed, r.failed = e.Elapsed, e.Action == "fail"
		}
	}
	if err := lines.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var packages, tests []*result
	for _, r := range results {
		if r.test == "" {
			packages = append(packages, r)
		} else if !strings.Contains(r.test, "/") {
			tests = append(tests, r)
		}
	}
	slowest := func(a, b *result) int {
		if a.elapsed != b.elapsed {
			if a.elapsed > b.elapsed {
				return -1
			}
			return 1
		}
		return strings.Compare(a.pkg+a.test, b.pkg+b.test)
	}
	slices.SortFunc(packages, slowest)
	slices.SortFunc(tests, slowest)
	for _, r := range tests {
		if r.failed {
			fmt.Printf("=== FAILED %s %s (%.2fs)\n%s\n", r.pkg, r.test, r.elapsed, r.output.String())
		}
	}
	for _, r := range packages {
		if r.failed {
			fmt.Printf("=== FAILED %s (%.2fs)\n%s\n", r.pkg, r.elapsed, r.output.String())
		}
	}
	fmt.Println("=== Slowest packages")
	for _, r := range packages[:min(len(packages), 8)] {
		fmt.Printf("%8.1fs %s\n", r.elapsed, r.pkg)
	}
	fmt.Println("=== Slowest tests")
	for _, r := range tests[:min(len(tests), 25)] {
		fmt.Printf("%8.1fs %s %s\n", r.elapsed, r.pkg, r.test)
	}
}
