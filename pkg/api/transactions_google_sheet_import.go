package api

import (
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
	fileData, err := fetcher.FetchCSV(c, sheetUrl)

	if err != nil {
		return nil, errs.Or(err, errs.ErrGoogleSheetFetchFailed)
	}

	dataImporter, err := converters.GetTransactionDataImporter(googleSheetImportFixedFileType)

	if err != nil {
		log.Errorf(c, "[transactions_google_sheet_import.TransactionParseGoogleSheetImportHandler] failed to get data importer for user \"uid:%d\", because %s", uid, err.Error())
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}

	additionalOptions := converter.ParseImporterOptions(a.CurrentConfig(), "")
	previewWrapper, previewErr := a.buildImportPreview(c, uid, dataImporter, fileData, clientTimezone, additionalOptions)

	if previewErr != nil {
		return nil, previewErr
	}

	duplicateReasons := a.detectGoogleSheetDuplicates(c, uid, previewWrapper.Items)
	items := make([]*models.GoogleSheetImportPreviewItem, len(previewWrapper.Items))
	duplicateCount := int64(0)

	for i := 0; i < len(previewWrapper.Items); i++ {
		reason := duplicateReasons[i]
		isDuplicate := reason != models.GOOGLE_SHEET_DUPLICATE_REASON_NONE

		if isDuplicate {
			duplicateCount++
		}

		items[i] = &models.GoogleSheetImportPreviewItem{
			ImportTransactionResponse: previewWrapper.Items[i],
			RowNumber:                 i + 1,
			IsDuplicate:               isDuplicate,
			DuplicateReason:           reason,
		}
	}

	return &models.GoogleSheetImportPreviewResponse{
		TotalRowCount:     previewWrapper.TotalCount,
		DuplicateRowCount: duplicateCount,
		Items:             items,
	}, nil
}

// detectGoogleSheetDuplicates returns a map from item index (within items) to the reason it was
// flagged as a duplicate, checking both duplicates within the sheet itself and duplicates against the
// user's existing transactions. There is no content-based duplicate detection anywhere else in
// ezbookkeeping today, so this is necessarily new logic - kept intentionally small and scoped only to
// this feature. It is a best-effort exact match on (type, time, category, account, amount, comment):
// nothing is silently excluded because of it, every flagged row still appears in the preview for the
// user to decide on before confirming the import.
func (a *TransactionsApi) detectGoogleSheetDuplicates(c *core.WebContext, uid int64, items []*models.ImportTransactionResponse) map[int]models.GoogleSheetDuplicateReason {
	duplicateReasons := markInSheetDuplicates(items)

	if len(items) < 1 {
		return duplicateReasons
	}

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

// markInSheetDuplicates is a pure, DB-independent function that flags rows duplicating an earlier row
// within the same parsed sheet, keyed on (type, time, category, account, amount, comment).
func markInSheetDuplicates(items []*models.ImportTransactionResponse) map[int]models.GoogleSheetDuplicateReason {
	duplicateReasons := make(map[int]models.GoogleSheetDuplicateReason, len(items))
	seenKeys := make(map[string]bool, len(items))

	for i := 0; i < len(items); i++ {
		item := items[i]
		key := googleSheetDuplicateKey(item.Type, item.Time, item.CategoryId, item.SourceAccountId, item.SourceAmount, item.Comment)

		if seenKeys[key] {
			duplicateReasons[i] = models.GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET
		} else {
			seenKeys[key] = true
		}
	}

	return duplicateReasons
}

// buildExistingGoogleSheetDuplicateKeys queries the user's existing transactions within the time
// range spanned by items (restricted to the accounts referenced by items) and returns their duplicate
// keys, reusing the existing GetTransactionsByMaxTime service function rather than a new query.
func (a *TransactionsApi) buildExistingGoogleSheetDuplicateKeys(c *core.WebContext, uid int64, items []*models.ImportTransactionResponse) (map[string]bool, error) {
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

	accountIds := make([]int64, 0, len(accountIdSet))

	for accountId := range accountIdSet {
		accountIds = append(accountIds, accountId)
	}

	existingTransactions, err := a.transactions.GetTransactionsByMaxTime(c, uid, maxTime+1, minTime-1, 0, nil, accountIds, nil, false, "", "", core.MATCH_MODE_DEFAULT, false, 1, maxGoogleSheetDuplicateCheckTransactionCount, false, true)

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
