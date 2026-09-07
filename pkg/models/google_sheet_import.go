package models

// GoogleSheetImportParseRequest represents all parameters of the Google Sheet import preview request
type GoogleSheetImportParseRequest struct {
	Url string `json:"url" binding:"required,max=2048"`
}

// GoogleSheetDuplicateReason represents why a parsed row was flagged as a duplicate
type GoogleSheetDuplicateReason string

// Google Sheet duplicate reasons
const (
	GOOGLE_SHEET_DUPLICATE_REASON_NONE               GoogleSheetDuplicateReason = ""
	GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET            GoogleSheetDuplicateReason = "in_sheet"
	GOOGLE_SHEET_DUPLICATE_REASON_EXISTING_TRANSACTION GoogleSheetDuplicateReason = "existing_transaction"
)

// GoogleSheetImportPreviewItem represents a single parsed transaction row in a Google Sheet import preview,
// annotated with its row number and whether it looks like a duplicate
type GoogleSheetImportPreviewItem struct {
	*ImportTransactionResponse
	RowNumber       int                        `json:"rowNumber"`
	IsDuplicate     bool                       `json:"isDuplicate"`
	DuplicateReason GoogleSheetDuplicateReason `json:"duplicateReason,omitempty"`
}

// GoogleSheetImportPreviewResponse represents the response of a Google Sheet import preview request
type GoogleSheetImportPreviewResponse struct {
	// SpreadsheetName and SheetName are best-effort display names parsed from the Google Sheets
	// export response; either may be empty if Google didn't provide them in a recognized shape.
	SpreadsheetName   string                          `json:"spreadsheetName,omitempty"`
	SheetName         string                          `json:"sheetName,omitempty"`
	TotalRowCount     int64                           `json:"totalRowCount"`
	DuplicateRowCount int64                           `json:"duplicateRowCount"`
	Items             []*GoogleSheetImportPreviewItem `json:"items"`
}
