package models

// GoogleSheetImportParseRequest represents all parameters of the Google Sheet import preview request
type GoogleSheetImportParseRequest struct {
	Url string `json:"url" binding:"required,max=2048"`
}

// GoogleSheetDuplicateReason represents why a parsed row was flagged as a duplicate
type GoogleSheetDuplicateReason string

// Google Sheet duplicate reasons
const (
	GOOGLE_SHEET_DUPLICATE_REASON_NONE                 GoogleSheetDuplicateReason = ""
	GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED     GoogleSheetDuplicateReason = "already_imported"
	GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET             GoogleSheetDuplicateReason = "in_sheet"
	GOOGLE_SHEET_DUPLICATE_REASON_EXISTING_TRANSACTION GoogleSheetDuplicateReason = "existing_transaction"
)

// GoogleSheetImportRowKey identifies one row of a Google Sheet for import-record purposes by its
// hashed content fingerprint. The row's position among identical rows is not part of it - the server
// assigns that when recording, because a client-supplied position shifts whenever a row is inserted
// or deleted.
type GoogleSheetImportRowKey struct {
	RowHash string `json:"rowHash" binding:"required,max=64"`
}

// GoogleSheetImportSource identifies the Google Sheet an import came from, so each imported row can
// be recorded and recognized on a later import of the same sheet. RowKeys is positional: RowKeys[i]
// describes the same row as Transactions[i] in the enclosing import request.
type GoogleSheetImportSource struct {
	SpreadsheetId string                     `json:"spreadsheetId" binding:"required,max=128"`
	Gid           string                     `json:"gid" binding:"max=32"`
	RowKeys       []*GoogleSheetImportRowKey `json:"rowKeys"`
}

// GoogleSheetImportPreviewItem represents a single parsed transaction row in a Google Sheet import preview,
// annotated with its row number, its row key and whether it looks like a duplicate
type GoogleSheetImportPreviewItem struct {
	*ImportTransactionResponse
	RowNumber       int                        `json:"rowNumber"`
	RowHash         string                     `json:"rowHash"`
	IsDuplicate     bool                       `json:"isDuplicate"`
	AlreadyImported bool                       `json:"alreadyImported"`
	DuplicateReason GoogleSheetDuplicateReason `json:"duplicateReason,omitempty"`
}

// GoogleSheetImportPreviewResponse represents the response of a Google Sheet import preview request
type GoogleSheetImportPreviewResponse struct {
	// SpreadsheetName and SheetName are best-effort display names parsed from the Google Sheets
	// export response; either may be empty if Google didn't provide them in a recognized shape.
	SpreadsheetName string `json:"spreadsheetName,omitempty"`
	SheetName       string `json:"sheetName,omitempty"`
	// SpreadsheetId and Gid identify the sheet, and are echoed back so the confirm request can
	// record which sheet each imported row came from.
	SpreadsheetId           string                          `json:"spreadsheetId"`
	Gid                     string                          `json:"gid"`
	TotalRowCount           int64                           `json:"totalRowCount"`
	DuplicateRowCount       int64                           `json:"duplicateRowCount"`
	AlreadyImportedRowCount int64                           `json:"alreadyImportedRowCount"`
	Items                   []*GoogleSheetImportPreviewItem `json:"items"`
}
