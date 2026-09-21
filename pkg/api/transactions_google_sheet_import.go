package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/mayswind/ezbookkeeping/pkg/converters"
	"github.com/mayswind/ezbookkeeping/pkg/converters/converter"
	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/googlesheet"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// googleSheetImportFixedFileType is the only import format Google Sheet import accepts in this
// version - the sheet must use ezBookkeeping's own native csv column layout, so the fetched data can
// be handed directly to the existing importer with no new parsing/mapping logic.
const googleSheetImportFixedFileType = "ezbookkeeping_csv"

// maxGoogleSheetDuplicateCheckTransactionCount caps how many existing transactions are pulled in a
// single duplicate-detection pass
const maxGoogleSheetDuplicateCheckTransactionCount = 20000

// TransactionParseGoogleSheetImportHandler fetches a public/shared Google Sheet, parses it with the
// existing ezbookkeeping csv importer, flags likely duplicates, and returns an import preview for the
// current user. It performs no persistence - confirming the import reuses the existing
// TransactionImportHandler ("/transactions/import.json") unchanged.
func (a *TransactionsApi) TransactionParseGoogleSheetImportHandler(c *core.WebContext) (any, *errs.Error) {
	uid := c.GetCurrentUid()

	var googleSheetImportReq models.GoogleSheetImportParseRequest
	err := c.ShouldBindJSON(&googleSheetImportReq)

	if err != nil {
		log.Warnf(c, "[transactions_google_sheet_import.TransactionParseGoogleSheetImportHandler] parse request failed for user \"uid:%d\", because %s", uid, err.Error())
		return nil, errs.NewIncompleteOrIncorrectSubmissionError(err)
	}

	clientTimezone, err := c.GetClientTimezone()

	if err != nil {
		log.Warnf(c, "[transactions_google_sheet_import.TransactionParseGoogleSheetImportHandler] cannot get client timezone, because %s", err.Error())
		return nil, errs.ErrClientTimezoneOffsetInvalid
	}

	googleSheetImportConfig := a.CurrentConfig().GoogleSheetImportConfig

	if googleSheetImportConfig == nil || !googleSheetImportConfig.Enabled {
		return nil, errs.ErrGoogleSheetImportNotEnabled
	}

	sheetUrl, err := googlesheet.ParseGoogleSheetURL(googleSheetImportReq.Url)

	if err != nil {
		return nil, errs.Or(err, errs.ErrGoogleSheetUrlInvalid)
	}

	fetcher := googlesheet.NewFetcher(googleSheetImportConfig)
	fetchResult, err := fetcher.FetchCSV(c, sheetUrl)

	if err != nil {
		return nil, errs.Or(err, errs.ErrGoogleSheetFetchFailed)
	}

	dataImporter, err := converters.GetTransactionDataImporter(googleSheetImportFixedFileType)

	if err != nil {
		log.Errorf(c, "[transactions_google_sheet_import.TransactionParseGoogleSheetImportHandler] failed to get data importer for user \"uid:%d\", because %s", uid, err.Error())
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}

	additionalOptions := converter.ParseImporterOptions(a.CurrentConfig(), "")
	previewWrapper, previewErr := a.buildImportPreview(c, uid, dataImporter, fetchResult.Data, clientTimezone, additionalOptions)

	if previewErr != nil {
		// Google Sheets silently auto-reformats pasted text that looks like a date/time (e.g.
		// "2026-09-08 00:04:53" becomes "08-09-2026  12.04.53 AM" under some locales), which the
		// importer then rejects as an invalid time. Surface a more actionable message for this
		// specific, common case rather than the generic time-invalid error.
		if previewErr == errs.ErrTransactionTimeInvalid || previewErr == errs.ErrTransactionTimeZoneInvalid {
			return nil, errs.ErrGoogleSheetTimeColumnInvalid
		}

		return nil, previewErr
	}

	rowKeys := computeGoogleSheetRowKeys(previewWrapper.Items)
	duplicateReasons := a.detectGoogleSheetDuplicates(c, uid, sheetUrl.SpreadsheetId, previewWrapper.Items, rowKeys)
	items := make([]*models.GoogleSheetImportPreviewItem, len(previewWrapper.Items))
	duplicateCount := int64(0)
	alreadyImportedCount := int64(0)

	for i := 0; i < len(previewWrapper.Items); i++ {
		reason := duplicateReasons[i]
		isDuplicate := reason != models.GOOGLE_SHEET_DUPLICATE_REASON_NONE
		alreadyImported := reason == models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED

		if isDuplicate {
			duplicateCount++
		}

		if alreadyImported {
			alreadyImportedCount++
		}

		items[i] = &models.GoogleSheetImportPreviewItem{
			ImportTransactionResponse: previewWrapper.Items[i],
			RowNumber:                 i + 1,
			RowHash:                   rowKeys[i].hash,
			IsDuplicate:               isDuplicate,
			AlreadyImported:           alreadyImported,
			DuplicateReason:           reason,
		}
	}

	return &models.GoogleSheetImportPreviewResponse{
		SpreadsheetName:         fetchResult.SpreadsheetName,
		SheetName:               fetchResult.SheetName,
		SpreadsheetId:           sheetUrl.SpreadsheetId,
		Gid:                     sheetUrl.GID,
		TotalRowCount:           previewWrapper.TotalCount,
		DuplicateRowCount:       duplicateCount,
		AlreadyImportedRowCount: alreadyImportedCount,
		Items:                   items,
	}, nil
}

// googleSheetRowKey identifies one parsed sheet row: hash is the content fingerprint, and occurrence
// distinguishes rows within the same sheet that share it (0 for the first row carrying that hash).
type googleSheetRowKey struct {
	hash       string
	occurrence int32
}

// computeGoogleSheetRowKeys is a pure, DB-independent function that assigns every parsed row its
// content hash and the occurrence index of that hash within the sheet.
func computeGoogleSheetRowKeys(items []*models.ImportTransactionResponse) []googleSheetRowKey {
	rowKeys := make([]googleSheetRowKey, len(items))
	occurrenceCounts := make(map[string]int32, len(items))

	for i := 0; i < len(items); i++ {
		item := items[i]
		hash := googleSheetRowHash(item.Type, item.Time, item.CategoryId, item.SourceAccountId, item.SourceAmount, item.Comment)

		rowKeys[i] = googleSheetRowKey{
			hash:       hash,
			occurrence: occurrenceCounts[hash],
		}

		occurrenceCounts[hash]++
	}

	return rowKeys
}

// detectGoogleSheetDuplicates returns a map from item index (within items) to the reason it was
// flagged as a duplicate. Three checks run in decreasing order of confidence:
//
//  1. already imported - this exact sheet row was imported before and the transaction it created
//     still exists. This is a recorded fact, not a guess.
//  2. in sheet - the row repeats an earlier row of the same sheet.
//  3. existing transaction - a transaction with the same content already exists. This is a
//     best-effort exact match on (type, time, category, account, amount, comment), and it stops
//     matching if the user edits the transaction afterwards - which is exactly why check 1 exists.
//
// Nothing is silently excluded: every flagged row is still returned in the preview, and the user
// decides what to import.
func (a *TransactionsApi) detectGoogleSheetDuplicates(c *core.WebContext, uid int64, spreadsheetId string, items []*models.ImportTransactionResponse, rowKeys []googleSheetRowKey) map[int]models.GoogleSheetDuplicateReason {
	duplicateReasons := make(map[int]models.GoogleSheetDuplicateReason, len(items))

	if len(items) < 1 {
		return duplicateReasons
	}

	importedRowHashCounts, err := a.buildAlreadyImportedGoogleSheetRowHashCounts(c, uid, spreadsheetId)

	if err != nil {
		log.Warnf(c, "[transactions_google_sheet_import.detectGoogleSheetDuplicates] failed to get import records for user \"uid:%d\", because %s", uid, err.Error())
	}

	markAlreadyImportedGoogleSheetRows(duplicateReasons, rowKeys, importedRowHashCounts)

	existingKeys, err := a.buildExistingGoogleSheetDuplicateKeys(c, uid, items)

	if err != nil {
		log.Warnf(c, "[transactions_google_sheet_import.detectGoogleSheetDuplicates] failed to get existing transactions for user \"uid:%d\" to check for duplicates, because %s", uid, err.Error())
		return duplicateReasons
	}

	for i := 0; i < len(items); i++ {
		if duplicateReasons[i] != models.GOOGLE_SHEET_DUPLICATE_REASON_NONE {
			continue
		}

		item := items[i]
		key := googleSheetDuplicateKey(item.Type, item.Time, item.CategoryId, item.SourceAccountId, item.SourceAmount, item.Comment)

		if existingKeys[key] {
			duplicateReasons[i] = models.GOOGLE_SHEET_DUPLICATE_REASON_EXISTING_TRANSACTION
		}
	}

	return duplicateReasons
}

// markAlreadyImportedGoogleSheetRows is a pure, DB-independent function that flags rows against the
// per-hash counts of rows already imported from this spreadsheet.
//
// It matches by count rather than by a row's position among identical rows: if the sheet holds three
// rows with one fingerprint and two of them were imported, the first two are flagged and the third is
// offered. Matching on position instead would break as soon as a row is inserted above or deleted from
// a run of identical rows, which silently un-flagged rows that really had been imported.
func markAlreadyImportedGoogleSheetRows(duplicateReasons map[int]models.GoogleSheetDuplicateReason, rowKeys []googleSheetRowKey, importedRowHashCounts map[string]int) {
	remainingCounts := make(map[string]int, len(importedRowHashCounts))

	for hash, count := range importedRowHashCounts {
		remainingCounts[hash] = count
	}

	for i := 0; i < len(rowKeys); i++ {
		hash := rowKeys[i].hash

		if remainingCounts[hash] > 0 {
			duplicateReasons[i] = models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED
			remainingCounts[hash]--
		} else if rowKeys[i].occurrence > 0 {
			duplicateReasons[i] = models.GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET
		}
	}
}

// buildAlreadyImportedGoogleSheetRowHashCounts returns, per row fingerprint, how many rows of this
// spreadsheet the user has already imported.
//
// The lookup deliberately ignores the sheet tab id. Google serves the same default tab whether the
// url carries a gid or not ("/edit?usp=sharing" parses to an empty gid, "/edit#gid=0" to "0"), so
// keying on gid meant the very same rows were recorded under one key and looked up under another,
// depending on which url form the user happened to paste.
//
// A record only counts while the transaction it created still exists: if the user deleted that
// transaction (individually, by clearing an account, or by clearing all data), the row must become
// importable again. Records whose transaction is gone are pruned here, so this stays self-healing
// without any of the delete paths needing to know about this table.
func (a *TransactionsApi) buildAlreadyImportedGoogleSheetRowHashCounts(c *core.WebContext, uid int64, spreadsheetId string) (map[string]int, error) {
	records, err := a.googleSheetImportRecords.GetImportRecordsBySpreadsheet(c, uid, spreadsheetId)

	if err != nil {
		return nil, err
	}

	if len(records) < 1 {
		return nil, nil
	}

	transactionIds := make([]int64, len(records))

	for i := 0; i < len(records); i++ {
		transactionIds[i] = records[i].TransactionId
	}

	existingTransactions, err := a.transactions.GetTransactionsByTransactionIds(c, uid, transactionIds)

	if err != nil {
		return nil, err
	}

	liveTransactionIds := make(map[int64]bool, len(existingTransactions))

	for i := 0; i < len(existingTransactions); i++ {
		liveTransactionIds[existingTransactions[i].TransactionId] = true
	}

	importedRowHashCounts := make(map[string]int, len(records))
	staleTransactionIds := make([]int64, 0, len(records))

	for i := 0; i < len(records); i++ {
		record := records[i]

		if liveTransactionIds[record.TransactionId] {
			importedRowHashCounts[record.RowHash]++
		} else {
			staleTransactionIds = append(staleTransactionIds, record.TransactionId)
		}
	}

	if len(staleTransactionIds) > 0 {
		if pruneErr := a.googleSheetImportRecords.DeleteImportRecordsByTransactionIds(c, uid, staleTransactionIds); pruneErr != nil {
			log.Warnf(c, "[transactions_google_sheet_import.buildAlreadyImportedGoogleSheetRowHashCounts] failed to prune %d stale import records for user \"uid:%d\", because %s", len(staleTransactionIds), uid, pruneErr.Error())
		}
	}

	return importedRowHashCounts, nil
}

// buildExistingGoogleSheetDuplicateKeys queries the user's existing transactions within the time
// range spanned by items (restricted to the accounts referenced by items) and returns their duplicate
// keys, reusing the existing GetTransactionsByMaxTime service function rather than a new query.
func (a *TransactionsApi) buildExistingGoogleSheetDuplicateKeys(c *core.WebContext, uid int64, items []*models.ImportTransactionResponse) (map[string]bool, error) {
	maxDbTransactionTime, minDbTransactionTime, accountIds := computeGoogleSheetDuplicateQueryBounds(items)

	existingTransactions, err := a.transactions.GetTransactionsByMaxTime(c, uid, maxDbTransactionTime, minDbTransactionTime, 0, nil, accountIds, nil, false, "", "", core.MATCH_MODE_DEFAULT, false, 1, maxGoogleSheetDuplicateCheckTransactionCount, false, true)

	if err != nil {
		return nil, err
	}

	existingKeys := make(map[string]bool, len(existingTransactions))

	for i := 0; i < len(existingTransactions); i++ {
		transaction := existingTransactions[i]
		transactionType, typeErr := transaction.Type.ToTransactionType()

		if typeErr != nil {
			continue
		}

		key := googleSheetDuplicateKey(transactionType, utils.GetUnixTimeFromTransactionTime(transaction.TransactionTime), transaction.CategoryId, transaction.AccountId, transaction.Amount, transaction.Comment)
		existingKeys[key] = true
	}

	return existingKeys, nil
}

func googleSheetDuplicateKey(transactionType models.TransactionType, unixTime int64, categoryId int64, accountId int64, amount int64, comment string) string {
	return fmt.Sprintf("%d|%d|%d|%d|%d|%s", transactionType, unixTime, categoryId, accountId, amount, comment)
}

// googleSheetRowHash hashes the duplicate key into a fixed-length fingerprint, so it fits a bounded
// database column regardless of how long the row's description is.
func googleSheetRowHash(transactionType models.TransactionType, unixTime int64, categoryId int64, accountId int64, amount int64, comment string) string {
	sum := sha256.Sum256([]byte(googleSheetDuplicateKey(transactionType, unixTime, categoryId, accountId, amount, comment)))
	return hex.EncodeToString(sum[:])
}

// validateGoogleSheetImportSource checks the Google Sheet source attached to an import request
// before any transaction is created. RowKeys is positional, so it must line up exactly with the
// submitted transactions.
func validateGoogleSheetImportSource(source *models.GoogleSheetImportSource, transactionCount int) *errs.Error {
	if source == nil {
		return nil
	}

	if len(source.RowKeys) != transactionCount {
		return errs.ErrGoogleSheetImportRowKeysInvalid
	}

	for i := 0; i < len(source.RowKeys); i++ {
		if source.RowKeys[i] == nil || source.RowKeys[i].RowHash == "" {
			return errs.ErrGoogleSheetImportRowKeysInvalid
		}
	}

	return nil
}

// buildGoogleSheetImportRecords is a pure function that pairs each created transaction with the sheet
// row it came from. It must run after the transaction ids have been assigned.
//
// The occurrence of each record is assigned here rather than taken from the client, continuing from
// the highest occurrence already stored for that fingerprint (nextOccurrences). A row's position
// among identical rows shifts whenever one of them is inserted or deleted, so trusting the client's
// position would both mis-record rows and risk colliding with an existing record.
func buildGoogleSheetImportRecords(uid int64, source *models.GoogleSheetImportSource, transactions []*models.Transaction, nextOccurrences map[string]int32, importedUnixTime int64) []*models.GoogleSheetImportRecord {
	if source == nil || len(source.RowKeys) != len(transactions) {
		return nil
	}

	if nextOccurrences == nil {
		nextOccurrences = make(map[string]int32)
	}

	records := make([]*models.GoogleSheetImportRecord, len(transactions))

	for i := 0; i < len(transactions); i++ {
		rowHash := source.RowKeys[i].RowHash

		records[i] = &models.GoogleSheetImportRecord{
			TransactionId:    transactions[i].TransactionId,
			Uid:              uid,
			SpreadsheetId:    source.SpreadsheetId,
			Gid:              source.Gid,
			RowHash:          rowHash,
			Occurrence:       nextOccurrences[rowHash],
			ImportedUnixTime: importedUnixTime,
		}

		nextOccurrences[rowHash]++
	}

	return records
}

// computeGoogleSheetDuplicateQueryBounds is a pure, DB-independent function that derives the
// GetTransactionsByMaxTime query bounds (in the DB's scaled transaction-time representation, not
// plain unix seconds) and the distinct account IDs referenced by items, spanning the whole set.
func computeGoogleSheetDuplicateQueryBounds(items []*models.ImportTransactionResponse) (maxDbTransactionTime int64, minDbTransactionTime int64, accountIds []int64) {
	minTime := items[0].Time
	maxTime := items[0].Time
	accountIdSet := make(map[int64]bool)

	for i := 0; i < len(items); i++ {
		item := items[i]

		if item.Time < minTime {
			minTime = item.Time
		}

		if item.Time > maxTime {
			maxTime = item.Time
		}

		accountIdSet[item.SourceAccountId] = true

		if item.DestinationAccountId != 0 {
			accountIdSet[item.DestinationAccountId] = true
		}
	}

	accountIds = make([]int64, 0, len(accountIdSet))

	for accountId := range accountIdSet {
		accountIds = append(accountIds, accountId)
	}

	// GetTransactionsByMaxTime compares against the stored transaction_time column, which is scaled
	// (unix time * 1000, plus a sub-second sequence) rather than plain unix seconds - convert the
	// unix-second bounds accordingly, or every real transaction time would compare as out of range.
	maxDbTransactionTime = utils.GetMaxTransactionTimeFromUnixTime(maxTime)
	minDbTransactionTime = utils.GetMinTransactionTimeFromUnixTime(minTime)

	return maxDbTransactionTime, minDbTransactionTime, accountIds
}
