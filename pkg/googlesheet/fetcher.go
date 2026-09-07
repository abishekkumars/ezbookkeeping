package googlesheet

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/httpclient"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
)

const maxRedirectCount = 3

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

		if req.URL.Scheme != "https" || !strings.EqualFold(req.URL.Hostname(), googleSheetHost) {
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

// FetchCSV fetches the csv content exported by the specified Google Sheet url
func (f *Fetcher) FetchCSV(ctx core.Context, sheetUrl *GoogleSheetURL) ([]byte, error) {
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

	return data, nil
}
