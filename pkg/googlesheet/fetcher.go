package googlesheet

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/httpclient"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
)

const maxRedirectCount = 3

// googleUserContentHostSuffix is Google's own CDN host for exported file content - a csv export
// request to docs.google.com legitimately 30x-redirects here to actually serve the data. This is
// still exclusively Google-controlled infrastructure (nobody else can obtain a subdomain of it), so
// allowing it does not reopen the SSRF gap the host allowlist exists to close.
const googleUserContentHostSuffix = ".googleusercontent.com"

// isAllowedRedirectHost reports whether a redirect target is safe to follow: either the original
// Google Sheets host, or Google's own user-content CDN that csv exports are actually served from.
func isAllowedRedirectHost(host string) bool {
	if strings.EqualFold(host, googleSheetHost) {
		return true
	}

	return len(host) > len(googleUserContentHostSuffix) && strings.HasSuffix(strings.ToLower(host), googleUserContentHostSuffix)
}

// Fetcher fetches csv data exported from a Google Sheet over a hardened http client
type Fetcher struct {
	httpClient      *http.Client
	maxResponseSize int64
	// buildRequestUrl is overridden in tests to point at a local httptest server instead of the
	// real Google host, so the fetcher's status/timeout/size-limit handling can be exercised
	// without any live network call.
	buildRequestUrl func(sheetUrl *GoogleSheetURL) string
}

// NewFetcher returns a new Google Sheet csv data fetcher according to the specified config
func NewFetcher(config *settings.GoogleSheetImportConfig) *Fetcher {
	// enableHttpResponseLog is always false here, regardless of the global debug log setting,
	// because that logging path reads and logs the full response body - and spreadsheet
	// contents must never be written to logs.
	httpClient := httpclient.NewHttpClient(config.RequestTimeout, config.Proxy, false, core.GetOutgoingUserAgent(), false)

	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirectCount {
			return errors.New("too many redirects")
		}

		if req.URL.Scheme != "https" || !isAllowedRedirectHost(req.URL.Hostname()) {
			return errors.New("redirected to a disallowed host")
		}

		return nil
	}

	return &Fetcher{
		httpClient:      httpClient,
		maxResponseSize: int64(config.MaxResponseSize),
		buildRequestUrl: func(sheetUrl *GoogleSheetURL) string {
			return sheetUrl.ExportCSVUrl()
		},
	}
}

// FetchResult holds the fetched csv data along with the spreadsheet/sheet display names Google
// includes on the export response, when available. SpreadsheetName and SheetName are best-effort
// only (parsed from a response header Google doesn't formally document the format of) and are never
// required for the import itself to succeed - they exist purely so callers can show the user a
// friendlier label than the raw url.
type FetchResult struct {
	Data            []byte
	SpreadsheetName string
	SheetName       string
}

// FetchCSV fetches the csv content exported by the specified Google Sheet url
func (f *Fetcher) FetchCSV(ctx core.Context, sheetUrl *GoogleSheetURL) (*FetchResult, error) {
	req, err := http.NewRequest(http.MethodGet, f.buildRequestUrl(sheetUrl), nil)

	if err != nil {
		log.Errorf(ctx, "[fetcher.FetchCSV] failed to build request for google sheet \"id:%s\", because %s", sheetUrl.SpreadsheetId, err.Error())
		return nil, errs.ErrGoogleSheetFetchFailed
	}

	req.Header.Set("Accept", "text/csv, text/plain, */*")

	resp, err := f.httpClient.Do(req)

	if err != nil {
		var urlErr interface{ Timeout() bool }

		if errors.As(err, &urlErr) && urlErr.Timeout() {
			log.Warnf(ctx, "[fetcher.FetchCSV] fetching google sheet \"id:%s\" timed out", sheetUrl.SpreadsheetId)
			return nil, errs.ErrGoogleSheetFetchTimeout
		}

		log.Errorf(ctx, "[fetcher.FetchCSV] failed to fetch google sheet \"id:%s\", because %s", sheetUrl.SpreadsheetId, err.Error())
		return nil, errs.ErrGoogleSheetFetchFailed
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		log.Warnf(ctx, "[fetcher.FetchCSV] google sheet \"id:%s\" is not accessible, response code is %d", sheetUrl.SpreadsheetId, resp.StatusCode)
		return nil, errs.ErrGoogleSheetNotAccessible
	}

	if resp.StatusCode != http.StatusOK {
		log.Errorf(ctx, "[fetcher.FetchCSV] failed to fetch google sheet \"id:%s\", response code is %d", sheetUrl.SpreadsheetId, resp.StatusCode)
		return nil, errs.ErrGoogleSheetFetchFailed
	}

	// A private/unshared sheet is often served as a 200 OK Google sign-in/permission HTML page
	// rather than an error status - reject it here rather than trying to parse it as CSV.
	if contentType := resp.Header.Get("Content-Type"); strings.Contains(contentType, "text/html") {
		log.Warnf(ctx, "[fetcher.FetchCSV] google sheet \"id:%s\" is not accessible, response content type is %s", sheetUrl.SpreadsheetId, contentType)
		return nil, errs.ErrGoogleSheetNotAccessible
	}

	limitedReader := io.LimitReader(resp.Body, f.maxResponseSize+1)
	data, err := io.ReadAll(limitedReader)

	if err != nil {
		log.Errorf(ctx, "[fetcher.FetchCSV] failed to read google sheet \"id:%s\" response body, because %s", sheetUrl.SpreadsheetId, err.Error())
		return nil, errs.ErrGoogleSheetFetchFailed
	}

	if int64(len(data)) > f.maxResponseSize {
		log.Warnf(ctx, "[fetcher.FetchCSV] google sheet \"id:%s\" response exceeds the maximum allowed size", sheetUrl.SpreadsheetId)
		return nil, errs.ErrGoogleSheetResponseTooLarge
	}

	if len(data) < 1 {
		return nil, errs.ErrGoogleSheetEmpty
	}

	spreadsheetName, sheetName := parseSpreadsheetAndSheetNameFromContentDisposition(resp.Header.Get("Content-Disposition"))

	return &FetchResult{
		Data:            data,
		SpreadsheetName: spreadsheetName,
		SheetName:       sheetName,
	}, nil
}

// parseSpreadsheetAndSheetNameFromContentDisposition extracts the spreadsheet name and sheet name
// from a csv export response's Content-Disposition header, which Google Sheets currently formats as
// filename*=UTF-8''"{spreadsheet name} - {sheet name}.csv". This is undocumented Google behavior, so
// parsing is best-effort: it returns empty strings (never an error) if the header is missing or
// doesn't match the expected shape, since this is purely a display nicety and must never block the
// actual import.
func parseSpreadsheetAndSheetNameFromContentDisposition(contentDisposition string) (spreadsheetName string, sheetName string) {
	filename := extractUtf8FilenameFromContentDisposition(contentDisposition)
	filename = strings.TrimSuffix(filename, ".csv")

	if filename == "" {
		return "", ""
	}

	const separator = " - "
	lastSeparatorIndex := strings.LastIndex(filename, separator)

	if lastSeparatorIndex < 0 {
		return filename, ""
	}

	return filename[:lastSeparatorIndex], filename[lastSeparatorIndex+len(separator):]
}

// extractUtf8FilenameFromContentDisposition returns the decoded value of the RFC 5987
// filename*=UTF-8''<percent-encoded> parameter of a Content-Disposition header, or "" if absent or
// malformed.
func extractUtf8FilenameFromContentDisposition(contentDisposition string) string {
	const marker = "filename*=UTF-8''"
	markerIndex := strings.Index(contentDisposition, marker)

	if markerIndex < 0 {
		return ""
	}

	value := contentDisposition[markerIndex+len(marker):]

	if semicolonIndex := strings.Index(value, ";"); semicolonIndex >= 0 {
		value = value[:semicolonIndex]
	}

	decoded, err := url.PathUnescape(strings.TrimSpace(value))

	if err != nil {
		return ""
	}

	return decoded
}
