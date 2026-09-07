package errs

import "net/http"

// Error codes related to Google Sheet transaction import
var (
	ErrGoogleSheetImportNotEnabled = NewNormalError(NormalSubcategoryGoogleSheetImport, 0, http.StatusBadRequest, "google sheet import is not enabled")
	ErrGoogleSheetUrlIsEmpty       = NewNormalError(NormalSubcategoryGoogleSheetImport, 1, http.StatusBadRequest, "google sheet url is empty")
	ErrGoogleSheetUrlInvalid       = NewNormalError(NormalSubcategoryGoogleSheetImport, 2, http.StatusBadRequest, "google sheet url is invalid")
	ErrGoogleSheetNotAccessible    = NewNormalError(NormalSubcategoryGoogleSheetImport, 3, http.StatusBadRequest, "google sheet is not accessible")
	ErrGoogleSheetFetchTimeout     = NewNormalError(NormalSubcategoryGoogleSheetImport, 4, http.StatusBadRequest, "fetching google sheet timed out")
	ErrGoogleSheetResponseTooLarge = NewNormalError(NormalSubcategoryGoogleSheetImport, 5, http.StatusBadRequest, "google sheet data exceeds the maximum allowed size")
	ErrGoogleSheetEmpty            = NewNormalError(NormalSubcategoryGoogleSheetImport, 6, http.StatusBadRequest, "no transaction data was found in the google sheet")
	ErrGoogleSheetFetchFailed      = NewNormalError(NormalSubcategoryGoogleSheetImport, 7, http.StatusBadRequest, "failed to fetch google sheet data")
)
