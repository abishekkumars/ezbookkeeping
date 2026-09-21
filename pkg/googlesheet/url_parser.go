package googlesheet

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/mayswind/ezbookkeeping/pkg/errs"
)

const googleSheetHost = "docs.google.com"
const googleSheetPathPrefix = "/spreadsheets/d/"

var spreadsheetIdPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var gidFragmentPattern = regexp.MustCompile(`gid=([0-9]+)`)
var numericPattern = regexp.MustCompile(`^[0-9]+$`)

// GoogleSheetURL represents a parsed and validated Google Sheets url
type GoogleSheetURL struct {
	SpreadsheetId string
	GID           string // empty means the default (first) sheet
}

// ParseGoogleSheetURL parses and validates a Google Sheets url, extracting the spreadsheet id and
// (optional) sheet gid. Only "https://docs.google.com/spreadsheets/d/{id}/..." urls are accepted -
// this is also the SSRF boundary for the feature, since the host is never taken from anywhere else.
func ParseGoogleSheetURL(rawUrl string) (*GoogleSheetURL, error) {
	if strings.TrimSpace(rawUrl) == "" {
		return nil, errs.ErrGoogleSheetUrlIsEmpty
	}

	parsedUrl, err := url.Parse(rawUrl)

	if err != nil {
		return nil, errs.ErrGoogleSheetUrlInvalid
	}

	if parsedUrl.Scheme != "https" {
		return nil, errs.ErrGoogleSheetUrlInvalid
	}

	if !strings.EqualFold(parsedUrl.Hostname(), googleSheetHost) {
		return nil, errs.ErrGoogleSheetUrlInvalid
	}

	if !strings.HasPrefix(parsedUrl.Path, googleSheetPathPrefix) {
		return nil, errs.ErrGoogleSheetUrlInvalid
	}

	remainingPath := strings.TrimPrefix(parsedUrl.Path, googleSheetPathPrefix)
	pathSegments := strings.SplitN(remainingPath, "/", 2)
	spreadsheetId := pathSegments[0]

	if spreadsheetId == "" || !spreadsheetIdPattern.MatchString(spreadsheetId) {
		return nil, errs.ErrGoogleSheetUrlInvalid
	}

	gid := parsedUrl.Query().Get("gid")

	if gid == "" && parsedUrl.Fragment != "" {
		if match := gidFragmentPattern.FindStringSubmatch(parsedUrl.Fragment); match != nil {
			gid = match[1]
		}
	}

	if gid != "" && !numericPattern.MatchString(gid) {
		return nil, errs.ErrGoogleSheetUrlInvalid
	}

	return &GoogleSheetURL{
		SpreadsheetId: spreadsheetId,
		GID:           gid,
	}, nil
}

// ExportCSVUrl returns the csv export url for this Google Sheet
func (u *GoogleSheetURL) ExportCSVUrl() string {
	exportUrl := fmt.Sprintf("https://%s/spreadsheets/d/%s/export?format=csv", googleSheetHost, u.SpreadsheetId)

	if u.GID != "" {
		exportUrl += "&gid=" + u.GID
	}

	return exportUrl
}
