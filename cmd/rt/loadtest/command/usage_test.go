package command

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// windowOf runs a usage command against a stub and returns the range it asked
// for, since the window is the only thing these three commands compute.
func windowOf(t *testing.T, route call, response any, args ...string) (start, end time.Time) {
	t.Helper()

	stub := serveLTM(t, map[call]any{route: response})
	expectSuccess(t, NewUsageCmd(), args...)

	query := stub.only().Query
	parse := func(key string) time.Time {
		t.Helper()
		millis, err := strconv.ParseInt(query.Get(key), 10, 64)
		if err != nil {
			t.Fatalf("%s = %q, want unix milliseconds: %v", key, query.Get(key), err)
		}
		return time.UnixMilli(millis).UTC()
	}
	return parse("startTime"), parse("endTime")
}

func anOverallUsage() map[string]any {
	return map[string]any{
		"accountId":    "acct1",
		"totalUsage":   1234.5,
		"serviceCount": 7,
		"ratioConfig": map[string]any{
			"accountId":         "acct1",
			"baseServiceWeight": 1.0,
			"metricWeights":     map[string]any{"peakUsers": 0.4, "runCount": 0.2},
		},
	}
}

func TestUsageGetReportsTheAccountTotal(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-service-usage/overall"): anOverallUsage(),
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewUsageCmd(), "get", "--last", "30d")

	mustContain(t, "usage get table", out.stdout, "acct1", "1234.50", "7")
}

// The weighting is what turns raw counters into the billed figure, so it has to
// survive to the JSON output even though the table has no room for it.
func TestUsageGetKeepsTheWeightingInTheJSON(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-service-usage/overall"): anOverallUsage(),
	})

	out := expectSuccess(t, NewUsageCmd(), "get", "--last", "30d")

	mustContain(t, "usage get json", out.stdout, "ratioConfig", "baseServiceWeight", "peakUsers")
}

func TestUsageDetailsBreaksDownByService(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-service-usage/details"): map[string]any{
			"accountId": "acct1",
			"total":     2,
			"services": []any{
				map[string]any{
					"serviceId": "checkout", "orgId": "default", "projectId": "perf",
					"runCount": 12, "peakUsers": 500, "maxWorkerCount": 3,
					"totalDurationSec": 7200, "totalVuSeconds": 3600000, "complexity": 88.25,
				},
				map[string]any{
					"serviceId": "search", "orgId": "default", "projectId": "perf",
					"runCount": 4, "peakUsers": 100, "maxWorkerCount": 1,
					"totalDurationSec": 1800, "totalVuSeconds": 180000, "complexity": 12.5,
				},
			},
		},
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewUsageCmd(), "details", "--last", "7d")

	mustContain(t, "usage details table", out.stdout,
		"SERVICE", "COMPLEXITY", "checkout", "88.25", "search", "12.50", "7200s")
}

// The service returns the report as rows with its own header, so the table
// takes that header rather than inventing column names that might not match.
func TestUsageReportTakesItsHeaderFromTheService(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-service-usage/report"): [][]string{
			{"Service", "Runs", "Complexity"},
			{"checkout", "12", "88.25"},
			{"TOTAL", "12", "88.25"},
		},
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewUsageCmd(), "report", "--last", "30d")

	mustContain(t, "usage report table", out.stdout, "Service", "Runs", "Complexity", "checkout", "TOTAL")
}

func TestUsageReportSurvivesAnEmptyReport(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-service-usage/report"): [][]string{},
	})

	globals().Format = formatTable
	expectSuccess(t, NewUsageCmd(), "report", "--last", "30d")
}

func TestUsageReportWritesCSVWhenGivenAPath(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-service-usage/report"): [][]string{
			{"Service", "Runs", "Complexity"},
			{"checkout, eu", "12", "88.25"},
			{"TOTAL", "12", "88.25"},
		},
	})

	destination := filepath.Join(t.TempDir(), "usage.csv")
	out := expectSuccess(t, NewUsageCmd(), "report", "--last", "30d", "--output-file", destination)

	file, err := os.Open(destination)
	if err != nil {
		t.Fatalf("opening the written report: %v", err)
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("the written report is not valid CSV: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("wrote %d rows, want 3", len(rows))
	}
	// A comma inside a field has to be quoted, which is the reason for going
	// through encoding/csv rather than joining the fields.
	if rows[1][0] != "checkout, eu" {
		t.Errorf("field with a comma round-tripped as %q", rows[1][0])
	}
	mustContain(t, "csv confirmation", out.stderr, "3 rows", destination)
}

func TestUsageReportReportsAPathItCannotWrite(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-service-usage/report"): [][]string{{"Service"}},
	})

	inaccessible := filepath.Join(t.TempDir(), "no-such-directory", "usage.csv")
	message := expectFailure(t, NewUsageCmd(), "report", "--last", "30d", "--output-file", inaccessible)

	mustContain(t, "csv write error", message, "creating", inaccessible)
}

// Both ends are required by the API, so --last exists to save naming them. It
// measures back from now, which --start also does, and honouring both would
// mean silently picking one.
func TestUsageRejectsLastAndStartTogether(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewUsageCmd(), "get", "--last", "30d", "--start", "2026-01-01")

	mustContain(t, "conflicting window error", message, "--last and --start are alternatives")
}

func TestUsageInsistsOnAWindow(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewUsageCmd(), "get")

	mustContain(t, "missing window error", message, "a time window is required", "--last 30d")
}

func TestUsageLastMeasuresBackFromNow(t *testing.T) {
	start, end := windowOf(t, GET("/load-service-usage/overall"), anOverallUsage(),
		"get", "--last", "30d")

	if got := end.Sub(start); got != 30*24*time.Hour {
		t.Errorf("window spans %v, want 30 days", got)
	}
	if drift := time.Since(end); drift < 0 || drift > time.Minute {
		t.Errorf("the window ends %v from now, want it to end at now", drift)
	}
}

// Days are how a usage window is spoken of; 720h is not how anyone says a
// month. Go's own duration syntax still works for the shorter ones.
func TestUsageLastAcceptsDaysHoursAndMinutes(t *testing.T) {
	for _, tc := range []struct {
		flag string
		want time.Duration
	}{
		{"30d", 30 * 24 * time.Hour},
		{"1d", 24 * time.Hour},
		{"0.5d", 12 * time.Hour},
		{"24h", 24 * time.Hour},
		{"90m", 90 * time.Minute},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			start, end := windowOf(t, GET("/load-service-usage/overall"), anOverallUsage(),
				"get", "--last", tc.flag)
			if got := end.Sub(start); got != tc.want {
				t.Errorf("--last %s spans %v, want %v", tc.flag, got, tc.want)
			}
		})
	}
}

func TestUsageRejectsADurationItCannotRead(t *testing.T) {
	for _, flag := range []string{"thirty days", "30", "-5d", "0d", "-1h", "0h"} {
		t.Run(flag, func(t *testing.T) {
			serveLTM(t, map[call]any{})
			message := expectFailure(t, NewUsageCmd(), "get", "--last", flag)
			mustContain(t, "duration error", message, "invalid --last")
		})
	}
}

// A date, a timestamp and unix millis all appear in this workflow: a date is
// what a human types, and millis is what the API itself reports, so a value
// read out of a response can be pasted straight back in.
func TestUsageStartAcceptsEveryFormItIsGiven(t *testing.T) {
	instant := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, form := range []string{
		"2026-01-01",
		"2026-01-01T00:00:00Z",
		strconv.FormatInt(instant.UnixMilli(), 10),
	} {
		t.Run(form, func(t *testing.T) {
			start, _ := windowOf(t, GET("/load-service-usage/overall"), anOverallUsage(),
				"get", "--start", form, "--end", "2026-02-01")
			if !start.Equal(instant) {
				t.Errorf("--start %s parsed as %s, want %s", form, start, instant)
			}
		})
	}
}

func TestUsageEndDefaultsToNow(t *testing.T) {
	_, end := windowOf(t, GET("/load-service-usage/overall"), anOverallUsage(),
		"get", "--start", "2026-01-01")

	if drift := time.Since(end); drift < 0 || drift > time.Minute {
		t.Errorf("the window ends %v from now, want now when --end is omitted", drift)
	}
}

func TestUsageRejectsATimeItCannotRead(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewUsageCmd(), "get", "--start", "last tuesday")

	mustContain(t, "time error", message, "invalid --start", "2026-01-01")
}

// --end is read before --start, so a mistake in it has to be named as --end
// rather than reported against the other flag.
func TestUsageRejectsAnEndItCannotRead(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewUsageCmd(), "get", "--start", "2026-01-01", "--end", "yesterday")

	mustContain(t, "time error", message, "invalid --end", "2026-01-01")
}

func TestUsageRejectsAnEndBeforeItsStart(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewUsageCmd(), "get", "--start", "2026-02-01", "--end", "2026-01-01")

	mustContain(t, "reversed window error", message, "ends before it starts")
}

// The endpoints read accountIdentifier alone. Sending an org or project would
// suggest the figures were narrowed by them, which they are not.
func TestUsageAsksForTheAccountAndNothingNarrower(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-service-usage/overall"): anOverallUsage(),
	})

	expectSuccess(t, NewUsageCmd(), "get", "--last", "30d")

	if got := stub.only().Query.Get("accountIdentifier"); got != "acct1" {
		t.Errorf("accountIdentifier = %q, want the account in scope", got)
	}
}
