package googlesheet

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
)

func TestIsAllowedRedirectHost_OriginalHost(t *testing.T) {
	assert.True(t, isAllowedRedirectHost("docs.google.com"))
	assert.True(t, isAllowedRedirectHost("DOCS.GOOGLE.COM"))
}

func TestIsAllowedRedirectHost_GoogleUserContentCDN(t *testing.T) {
	// This is the real redirect target docs.google.com/.../export?format=csv sends the actual file
	// content from - confirmed by manually testing this feature against a live Google Sheet, which
	// is exactly how this case was caught: the fetcher initially rejected it as "disallowed host".
	assert.True(t, isAllowedRedirectHost("doc-08-34-sheets.googleusercontent.com"))
	assert.True(t, isAllowedRedirectHost("lh3.googleusercontent.com"))
}

func TestIsAllowedRedirectHost_RejectsUnrelatedAndSpoofedHosts(t *testing.T) {
	disallowedHosts := []string{
		"evil.com",
		"googleusercontent.com.evil.com",
		"evilgoogleusercontent.com",
		"127.0.0.1",
		"localhost",
		"",
	}

	for _, host := range disallowedHosts {
		assert.False(t, isAllowedRedirectHost(host), "host %q should not be allowed", host)
	}
}

func newTestFetcher(t *testing.T, server *httptest.Server, maxResponseSize int64, timeout time.Duration) *Fetcher {
	t.Cleanup(server.Close)

	return &Fetcher{
		httpClient:      &http.Client{Timeout: timeout},
		maxResponseSize: maxResponseSize,
		buildRequestUrl: func(sheetUrl *GoogleSheetURL) string {
			return server.URL
		},
	}
}

func TestFetcher_FetchCSV_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Time,Type,Amount\n2026-01-01 00:00:00,Expense,100\n"))
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	result, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Nil(t, err)
	assert.True(t, strings.Contains(string(result.Data), "Expense"))
}

func TestFetcher_FetchCSV_ParsesSpreadsheetAndSheetNameFromContentDisposition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="MyBudget-Sheet1.csv"; filename*=UTF-8''My%20Budget%20-%20Sheet1.csv`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Time,Type,Amount\n2026-01-01 00:00:00,Expense,100\n"))
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	result, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Nil(t, err)
	assert.Equal(t, "My Budget", result.SpreadsheetName)
	assert.Equal(t, "Sheet1", result.SheetName)
}

func TestFetcher_FetchCSV_MissingContentDispositionLeavesNamesEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Time,Type,Amount\n2026-01-01 00:00:00,Expense,100\n"))
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	result, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Nil(t, err)
	assert.Equal(t, "", result.SpreadsheetName)
	assert.Equal(t, "", result.SheetName)
}

func TestParseSpreadsheetAndSheetNameFromContentDisposition(t *testing.T) {
	spreadsheetName, sheetName := parseSpreadsheetAndSheetNameFromContentDisposition(
		`attachment; filename="Untitledspreadsheet-Sheet1.csv"; filename*=UTF-8''Untitled%20spreadsheet%20-%20Sheet1.csv`)

	assert.Equal(t, "Untitled spreadsheet", spreadsheetName)
	assert.Equal(t, "Sheet1", sheetName)
}

func TestParseSpreadsheetAndSheetNameFromContentDisposition_NameContainsSeparator(t *testing.T) {
	// The spreadsheet's own name can itself contain " - "; splitting on the LAST occurrence keeps
	// that intact and still isolates the (much shorter, simpler) sheet/tab name correctly.
	spreadsheetName, sheetName := parseSpreadsheetAndSheetNameFromContentDisposition(
		`attachment; filename*=UTF-8''My%20Budget%20-%202026%20-%20Sheet1.csv`)

	assert.Equal(t, "My Budget - 2026", spreadsheetName)
	assert.Equal(t, "Sheet1", sheetName)
}

func TestParseSpreadsheetAndSheetNameFromContentDisposition_NoSeparator(t *testing.T) {
	spreadsheetName, sheetName := parseSpreadsheetAndSheetNameFromContentDisposition(
		`attachment; filename*=UTF-8''Untitled.csv`)

	assert.Equal(t, "Untitled", spreadsheetName)
	assert.Equal(t, "", sheetName)
}

func TestParseSpreadsheetAndSheetNameFromContentDisposition_Empty(t *testing.T) {
	spreadsheetName, sheetName := parseSpreadsheetAndSheetNameFromContentDisposition("")

	assert.Equal(t, "", spreadsheetName)
	assert.Equal(t, "", sheetName)
}

func TestParseSpreadsheetAndSheetNameFromContentDisposition_MalformedEscape(t *testing.T) {
	spreadsheetName, sheetName := parseSpreadsheetAndSheetNameFromContentDisposition(
		`attachment; filename*=UTF-8''Bad%ZZName.csv`)

	assert.Equal(t, "", spreadsheetName)
	assert.Equal(t, "", sheetName)
}

func TestFetcher_FetchCSV_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	_, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Equal(t, errs.ErrGoogleSheetNotAccessible, err)
}

func TestFetcher_FetchCSV_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	_, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Equal(t, errs.ErrGoogleSheetNotAccessible, err)
}

func TestFetcher_FetchCSV_PrivateSheetServedAsHtml(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Sign in to continue</body></html>"))
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	_, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Equal(t, errs.ErrGoogleSheetNotAccessible, err)
}

func TestFetcher_FetchCSV_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	_, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Equal(t, errs.ErrGoogleSheetFetchFailed, err)
}

func TestFetcher_FetchCSV_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	fetcher := newTestFetcher(t, server, 1024, 20*time.Millisecond)
	_, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Equal(t, errs.ErrGoogleSheetFetchTimeout, err)
}

func TestFetcher_FetchCSV_OversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("a", 2048)))
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	_, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Equal(t, errs.ErrGoogleSheetResponseTooLarge, err)
}

func TestFetcher_FetchCSV_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	_, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Equal(t, errs.ErrGoogleSheetEmpty, err)
}

func TestFetcher_FetchCSV_InvalidCSVIsStillReturnedForImporterToValidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not,a,valid,ezbookkeeping,csv,file"))
	}))

	fetcher := newTestFetcher(t, server, 1024, time.Second)
	result, err := fetcher.FetchCSV(core.NewNullContext(), &GoogleSheetURL{SpreadsheetId: "abc"})

	assert.Nil(t, err)
	assert.NotEmpty(t, result.Data)
}
