package main

import (
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/fieldalignment"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"golang.org/x/tools/go/analysis/passes/structtag"

	"github.com/agnivade/sqlargs"

	"github.com/kisielk/errcheck/errcheck"

	"honnef.co/go/tools/staticcheck"
)

func main() {
	checks := []*analysis.Analyzer{
		atomic.Analyzer,
		errcheck.Analyzer,
		fieldalignment.Analyzer,
		httpresponse.Analyzer,
		printf.Analyzer,
		shadow.Analyzer,
		structtag.Analyzer,
		sqlargs.Analyzer,
		OsExitAnalyzer,
	}

	stChecks := map[string]bool{
		"ST1001": true,
		"QF1007": true,
		"QF1009": true,
		"QF1011": true,
	}

	for _, v := range staticcheck.Analyzers {
		if stChecks[v.Name] || (strings.HasPrefix(v.Name, "SA") && !strings.HasPrefix(v.Name, "SA4")) {
			checks = append(checks, v)
		}
	}
	multichecker.Main(
		checks...,
	)
}
