package models

// GoogleSheetImportRecord represents one Google Sheet row that a user has already imported.
//
// Content-based duplicate detection can only answer "does a transaction that looks like this exist",
// which stops matching as soon as the user edits the imported transaction, and cannot tell a deleted
// transaction apart from one that was never imported. This table answers the stronger question
// "was this exact sheet row imported", so a re-fetch of the same sheet can hide rows that are
// already in the books.
type GoogleSheetImportRecord struct {
	// TransactionId is the transaction this row created, and doubles as the primary key - it is
	// already a globally unique id, so no separate uuid type is needed for this table.
	TransactionId    int64  `xorm:"PK"`
	Uid              int64  `xorm:"UNIQUE(UQE_gs_import_record_uid_sheet_row) INDEX(IDX_gs_import_record_uid_sheet) NOT NULL"`
	SpreadsheetId    string `xorm:"UNIQUE(UQE_gs_import_record_uid_sheet_row) INDEX(IDX_gs_import_record_uid_sheet) VARCHAR(128) NOT NULL"`
	Gid              string `xorm:"UNIQUE(UQE_gs_import_record_uid_sheet_row) INDEX(IDX_gs_import_record_uid_sheet) VARCHAR(32) NOT NULL"`
	RowHash          string `xorm:"UNIQUE(UQE_gs_import_record_uid_sheet_row) VARCHAR(64) NOT NULL"`
	// Occurrence distinguishes rows that are byte-for-byte identical within the same sheet (the nth
	// row carrying this hash), so two legitimately identical transactions can both be recorded.
	Occurrence       int32 `xorm:"UNIQUE(UQE_gs_import_record_uid_sheet_row) NOT NULL"`
	ImportedUnixTime int64 `xorm:"NOT NULL"`
}
