package googlesheet

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/errs"
)

func TestParseGoogleSheetURL_EditUrl(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit")
	assert.Nil(t, err)
	assert.Equal(t, "1AbCdEfGhIjKlMnOpQrStUvWxYz", actualValue.SpreadsheetId)
	assert.Equal(t, "", actualValue.GID)
	assert.Equal(t, "https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/export?format=csv", actualValue.ExportCSVUrl())
}

func TestParseGoogleSheetURL_EditUrlWithGID(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit#gid=123456")
	assert.Nil(t, err)
	assert.Equal(t, "1AbCdEfGhIjKlMnOpQrStUvWxYz", actualValue.SpreadsheetId)
	assert.Equal(t, "123456", actualValue.GID)
	assert.Equal(t, "https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/export?format=csv&gid=123456", actualValue.ExportCSVUrl())
}

func TestParseGoogleSheetURL_UrlWithExtraQueryParameters(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit?usp=sharing&gid=987")
	assert.Nil(t, err)
	assert.Equal(t, "1AbCdEfGhIjKlMnOpQrStUvWxYz", actualValue.SpreadsheetId)
	assert.Equal(t, "987", actualValue.GID)
}

func TestParseGoogleSheetURL_ExportUrl(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/export?format=csv&gid=42")
	assert.Nil(t, err)
	assert.Equal(t, "1AbCdEfGhIjKlMnOpQrStUvWxYz", actualValue.SpreadsheetId)
	assert.Equal(t, "42", actualValue.GID)
}

func TestParseGoogleSheetURL_InvalidGoogleUrl(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("https://docs.google.com/document/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit")
	assert.Nil(t, actualValue)
	assert.Equal(t, errs.ErrGoogleSheetUrlInvalid, err)
}

func TestParseGoogleSheetURL_NonGoogleUrl(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("https://example.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit")
	assert.Nil(t, actualValue)
	assert.Equal(t, errs.ErrGoogleSheetUrlInvalid, err)
}

func TestParseGoogleSheetURL_MalformedSpreadsheetId(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("https://docs.google.com/spreadsheets/d/invalid id with spaces/edit")
	assert.Nil(t, actualValue)
	assert.Equal(t, errs.ErrGoogleSheetUrlInvalid, err)
}

func TestParseGoogleSheetURL_MissingSpreadsheetId(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("https://docs.google.com/spreadsheets/d/")
	assert.Nil(t, actualValue)
	assert.Equal(t, errs.ErrGoogleSheetUrlInvalid, err)

	actualValue, err = ParseGoogleSheetURL("https://docs.google.com/spreadsheets/d")
	assert.Nil(t, actualValue)
	assert.Equal(t, errs.ErrGoogleSheetUrlInvalid, err)
}

func TestParseGoogleSheetURL_EmptyUrl(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("")
	assert.Nil(t, actualValue)
	assert.Equal(t, errs.ErrGoogleSheetUrlIsEmpty, err)
}

func TestParseGoogleSheetURL_NonHttpsScheme(t *testing.T) {
	actualValue, err := ParseGoogleSheetURL("http://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit")
	assert.Nil(t, actualValue)
	assert.Equal(t, errs.ErrGoogleSheetUrlInvalid, err)
}

func TestParseGoogleSheetURL_SSRFAttempts(t *testing.T) {
	ssrfUrls := []string{
		"https://127.0.0.1/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit",
		"https://localhost/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit",
		"https://192.168.1.1/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit",
		"https://10.0.0.1/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit",
		"https://172.16.0.1/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit",
		"https://169.254.169.254/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit",
		"https://docs.google.com.evil.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit",
		"https://docs.google.com@evil.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/edit",
		"file:///etc/passwd",
	}

	for _, ssrfUrl := range ssrfUrls {
		actualValue, err := ParseGoogleSheetURL(ssrfUrl)
		assert.Nil(t, actualValue, "url %q should have been rejected", ssrfUrl)
		assert.Equal(t, errs.ErrGoogleSheetUrlInvalid, err, "url %q should have been rejected", ssrfUrl)
	}
}
