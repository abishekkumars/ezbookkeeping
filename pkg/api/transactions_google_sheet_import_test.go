package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

func newTestGoogleSheetImportItem(transactionTime int64, categoryId int64, accountId int64, amount int64, comment string) *models.ImportTransactionResponse {
	return &models.ImportTransactionResponse{
		Type:            models.TRANSACTION_TYPE_EXPENSE,
		CategoryId:      categoryId,
		Time:            transactionTime,
		SourceAccountId: accountId,
		SourceAmount:    amount,
		Comment:         comment,
	}
}

func occurrencesOf(rowKeys []googleSheetRowKey) []int32 {
	occurrences := make([]int32, len(rowKeys))

	for i := 0; i < len(rowKeys); i++ {
		occurrences[i] = rowKeys[i].occurrence
	}

	return occurrences
}

func TestComputeGoogleSheetRowKeys_NoDuplicates(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(2000, 1, 1, 200, "dinner"),
		newTestGoogleSheetImportItem(3000, 2, 1, 300, "groceries"),
	}

	actualValue := computeGoogleSheetRowKeys(items)

	assert.Equal(t, []int32{0, 0, 0}, occurrencesOf(actualValue))
	assert.NotEqual(t, actualValue[0].hash, actualValue[1].hash)
	assert.NotEqual(t, actualValue[1].hash, actualValue[2].hash)
}

func TestComputeGoogleSheetRowKeys_ExactDuplicateRowGetsNextOccurrence(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
	}

	actualValue := computeGoogleSheetRowKeys(items)

	assert.Equal(t, actualValue[0].hash, actualValue[1].hash)
	assert.Equal(t, []int32{0, 1}, occurrencesOf(actualValue))
}

func TestComputeGoogleSheetRowKeys_DifferentAmountIsNotADuplicate(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 150, "lunch"),
	}

	actualValue := computeGoogleSheetRowKeys(items)

	assert.NotEqual(t, actualValue[0].hash, actualValue[1].hash)
	assert.Equal(t, []int32{0, 0}, occurrencesOf(actualValue))
}

func TestComputeGoogleSheetRowKeys_DifferentCommentIsNotADuplicate(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "dinner"),
	}

	actualValue := computeGoogleSheetRowKeys(items)

	assert.NotEqual(t, actualValue[0].hash, actualValue[1].hash)
	assert.Equal(t, []int32{0, 0}, occurrencesOf(actualValue))
}

func TestComputeGoogleSheetRowKeys_MultipleDuplicatesOfSameRowCountUpIndependently(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(2000, 1, 1, 200, "dinner"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
	}

	actualValue := computeGoogleSheetRowKeys(items)

	assert.Equal(t, []int32{0, 0, 1, 2}, occurrencesOf(actualValue))
}

func TestComputeGoogleSheetRowKeys_EmptyItems(t *testing.T) {
	actualValue := computeGoogleSheetRowKeys([]*models.ImportTransactionResponse{})

	assert.Empty(t, actualValue)
}

func TestGoogleSheetRowHash_IsFixedLengthRegardlessOfCommentLength(t *testing.T) {
	// The hash is stored in a bounded database column, so a very long description must not overflow
	// it - and a truncated description must not collide with the untruncated one.
	shortHash := googleSheetRowHash(models.TRANSACTION_TYPE_EXPENSE, 1000, 1, 1, 100, "lunch")
	longHash := googleSheetRowHash(models.TRANSACTION_TYPE_EXPENSE, 1000, 1, 1, 100, strings.Repeat("a", 4096))

	assert.Len(t, shortHash, 64)
	assert.Len(t, longHash, 64)
	assert.NotEqual(t, shortHash, longHash)
}

func markedReasons(rowKeys []googleSheetRowKey, importedRowHashCounts map[string]int) []models.GoogleSheetDuplicateReason {
	duplicateReasons := make(map[int]models.GoogleSheetDuplicateReason, len(rowKeys))
	markAlreadyImportedGoogleSheetRows(duplicateReasons, rowKeys, importedRowHashCounts)

	reasons := make([]models.GoogleSheetDuplicateReason, len(rowKeys))

	for i := 0; i < len(rowKeys); i++ {
		reasons[i] = duplicateReasons[i]
	}

	return reasons
}

func TestMarkAlreadyImportedGoogleSheetRows_AppendingANewRowKeepsEarlierRowsFlagged(t *testing.T) {
	// The reported bug: every row had been imported, then one new row was added to the sheet, and a
	// previously imported row came back as importable.
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(2000, 1, 1, 200, "dinner"),
		newTestGoogleSheetImportItem(3000, 1, 1, 210, "fruits"),
	}

	importedRowKeys := computeGoogleSheetRowKeys(items)
	importedRowHashCounts := map[string]int{
		importedRowKeys[0].hash: 1,
		importedRowKeys[1].hash: 1,
		importedRowKeys[2].hash: 1,
	}

	itemsAfterAppend := append(items, newTestGoogleSheetImportItem(4000, 1, 1, 1, "test"))
	actualValue := markedReasons(computeGoogleSheetRowKeys(itemsAfterAppend), importedRowHashCounts)

	assert.Equal(t, []models.GoogleSheetDuplicateReason{
		models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED,
		models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED,
		models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED,
		models.GOOGLE_SHEET_DUPLICATE_REASON_NONE,
	}, actualValue)
}

func TestMarkAlreadyImportedGoogleSheetRows_InsertingADuplicateRowDoesNotUnflagTheImportedOne(t *testing.T) {
	// Two identical rows were imported. A third identical row is then inserted between them, which
	// shifts every position-based index - the imported rows must stay flagged regardless.
	importedItems := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
	}

	hash := computeGoogleSheetRowKeys(importedItems)[0].hash
	importedRowHashCounts := map[string]int{hash: 2}

	itemsAfterInsert := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
	}

	actualValue := markedReasons(computeGoogleSheetRowKeys(itemsAfterInsert), importedRowHashCounts)

	assert.Equal(t, []models.GoogleSheetDuplicateReason{
		models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED,
		models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED,
		models.GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET,
	}, actualValue)
}

func TestMarkAlreadyImportedGoogleSheetRows_DeletingARowDoesNotUnflagTheRest(t *testing.T) {
	importedItems := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(2000, 1, 1, 200, "dinner"),
		newTestGoogleSheetImportItem(3000, 1, 1, 300, "groceries"),
	}

	importedRowKeys := computeGoogleSheetRowKeys(importedItems)
	importedRowHashCounts := map[string]int{
		importedRowKeys[0].hash: 1,
		importedRowKeys[1].hash: 1,
		importedRowKeys[2].hash: 1,
	}

	itemsAfterDelete := []*models.ImportTransactionResponse{importedItems[0], importedItems[2]}
	actualValue := markedReasons(computeGoogleSheetRowKeys(itemsAfterDelete), importedRowHashCounts)

	assert.Equal(t, []models.GoogleSheetDuplicateReason{
		models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED,
		models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED,
	}, actualValue)
}

func TestMarkAlreadyImportedGoogleSheetRows_OnlySomeOfTheIdenticalRowsImported(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
	}

	rowKeys := computeGoogleSheetRowKeys(items)
	actualValue := markedReasons(rowKeys, map[string]int{rowKeys[0].hash: 2})

	assert.Equal(t, []models.GoogleSheetDuplicateReason{
		models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED,
		models.GOOGLE_SHEET_DUPLICATE_REASON_ALREADY_IMPORTED,
		models.GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET,
	}, actualValue)
}

func TestMarkAlreadyImportedGoogleSheetRows_NothingImportedYet(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
	}

	actualValue := markedReasons(computeGoogleSheetRowKeys(items), nil)

	assert.Equal(t, []models.GoogleSheetDuplicateReason{
		models.GOOGLE_SHEET_DUPLICATE_REASON_NONE,
		models.GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET,
	}, actualValue)
}

func TestValidateGoogleSheetImportSource(t *testing.T) {
	assert.Nil(t, validateGoogleSheetImportSource(nil, 3))

	validSource := &models.GoogleSheetImportSource{
		SpreadsheetId: "sheet-id",
		Gid:           "0",
		RowKeys: []*models.GoogleSheetImportRowKey{
			{RowHash: "hash-a"},
			{RowHash: "hash-a"},
		},
	}

	assert.Nil(t, validateGoogleSheetImportSource(validSource, 2))
	assert.Equal(t, errs.ErrGoogleSheetImportRowKeysInvalid, validateGoogleSheetImportSource(validSource, 1))

	emptyHashSource := &models.GoogleSheetImportSource{
		SpreadsheetId: "sheet-id",
		RowKeys:       []*models.GoogleSheetImportRowKey{{RowHash: ""}},
	}

	assert.Equal(t, errs.ErrGoogleSheetImportRowKeysInvalid, validateGoogleSheetImportSource(emptyHashSource, 1))

	nilRowKeySource := &models.GoogleSheetImportSource{
		SpreadsheetId: "sheet-id",
		RowKeys:       []*models.GoogleSheetImportRowKey{nil},
	}

	assert.Equal(t, errs.ErrGoogleSheetImportRowKeysInvalid, validateGoogleSheetImportSource(nilRowKeySource, 1))
}

func TestBuildGoogleSheetImportRecords_PairsEachTransactionWithItsRow(t *testing.T) {
	source := &models.GoogleSheetImportSource{
		SpreadsheetId: "sheet-id",
		Gid:           "7",
		RowKeys: []*models.GoogleSheetImportRowKey{
			{RowHash: "hash-a"},
			{RowHash: "hash-a"},
		},
	}

	transactions := []*models.Transaction{
		{TransactionId: 11},
		{TransactionId: 22},
	}

	actualValue := buildGoogleSheetImportRecords(1234, source, transactions, nil, 99)

	assert.Len(t, actualValue, 2)
	assert.Equal(t, int64(11), actualValue[0].TransactionId)
	assert.Equal(t, int64(22), actualValue[1].TransactionId)
	assert.Equal(t, int64(1234), actualValue[0].Uid)
	assert.Equal(t, "sheet-id", actualValue[0].SpreadsheetId)
	assert.Equal(t, "7", actualValue[0].Gid)
	assert.Equal(t, "hash-a", actualValue[0].RowHash)
	assert.Equal(t, int32(0), actualValue[0].Occurrence)
	assert.Equal(t, int32(1), actualValue[1].Occurrence)
	assert.Equal(t, int64(99), actualValue[0].ImportedUnixTime)
}

func TestBuildGoogleSheetImportRecords_ContinuesOccurrencesFromWhatIsAlreadyStored(t *testing.T) {
	// Importing one of two identical rows now and the other later must not reuse an occurrence that
	// is already taken, or the second import would collide on the unique index and roll back.
	source := &models.GoogleSheetImportSource{
		SpreadsheetId: "sheet-id",
		RowKeys:       []*models.GoogleSheetImportRowKey{{RowHash: "hash-a"}},
	}

	actualValue := buildGoogleSheetImportRecords(1234, source, []*models.Transaction{{TransactionId: 33}}, map[string]int32{"hash-a": 2}, 99)

	assert.Len(t, actualValue, 1)
	assert.Equal(t, int32(2), actualValue[0].Occurrence)
}

func TestBuildGoogleSheetImportRecords_ReturnsNothingWhenCountsDoNotLineUp(t *testing.T) {
	source := &models.GoogleSheetImportSource{
		SpreadsheetId: "sheet-id",
		RowKeys:       []*models.GoogleSheetImportRowKey{{RowHash: "hash-a"}},
	}

	assert.Nil(t, buildGoogleSheetImportRecords(1234, source, []*models.Transaction{{TransactionId: 11}, {TransactionId: 22}}, nil, 99))
	assert.Nil(t, buildGoogleSheetImportRecords(1234, nil, []*models.Transaction{{TransactionId: 11}}, nil, 99))
}

func TestGoogleSheetDuplicateKey_DistinctInputsProduceDistinctKeys(t *testing.T) {
	baseKey := googleSheetDuplicateKey(models.TRANSACTION_TYPE_EXPENSE, 1000, 1, 1, 100, "lunch")

	assert.NotEqual(t, baseKey, googleSheetDuplicateKey(models.TRANSACTION_TYPE_INCOME, 1000, 1, 1, 100, "lunch"))
	assert.NotEqual(t, baseKey, googleSheetDuplicateKey(models.TRANSACTION_TYPE_EXPENSE, 1001, 1, 1, 100, "lunch"))
	assert.NotEqual(t, baseKey, googleSheetDuplicateKey(models.TRANSACTION_TYPE_EXPENSE, 1000, 2, 1, 100, "lunch"))
	assert.NotEqual(t, baseKey, googleSheetDuplicateKey(models.TRANSACTION_TYPE_EXPENSE, 1000, 1, 2, 100, "lunch"))
	assert.NotEqual(t, baseKey, googleSheetDuplicateKey(models.TRANSACTION_TYPE_EXPENSE, 1000, 1, 1, 101, "lunch"))
	assert.NotEqual(t, baseKey, googleSheetDuplicateKey(models.TRANSACTION_TYPE_EXPENSE, 1000, 1, 1, 100, "dinner"))
	assert.Equal(t, baseKey, googleSheetDuplicateKey(models.TRANSACTION_TYPE_EXPENSE, 1000, 1, 1, 100, "lunch"))
}

func TestComputeGoogleSheetDuplicateQueryBounds_ConvertsUnixSecondsToDbTransactionTimeScale(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
	}

	maxDbTransactionTime, minDbTransactionTime, _ := computeGoogleSheetDuplicateQueryBounds(items)

	// The DB transaction_time column is scaled (unix time * 1000, plus a sub-second sequence), not
	// plain unix seconds - this is the exact bug class that made existing-transaction duplicate
	// detection a silent no-op, so pin the expected scaled values explicitly rather than just
	// checking they differ from the raw unix time.
	assert.Equal(t, utils.GetMinTransactionTimeFromUnixTime(1000), minDbTransactionTime)
	assert.Equal(t, utils.GetMaxTransactionTimeFromUnixTime(1000), maxDbTransactionTime)
	assert.Equal(t, int64(1000000), minDbTransactionTime)
	assert.Equal(t, int64(1000999), maxDbTransactionTime)
}

func TestComputeGoogleSheetDuplicateQueryBounds_SpansMinAndMaxAcrossItems(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(5000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 200, "dinner"),
		newTestGoogleSheetImportItem(3000, 1, 1, 300, "groceries"),
	}

	maxDbTransactionTime, minDbTransactionTime, _ := computeGoogleSheetDuplicateQueryBounds(items)

	assert.Equal(t, utils.GetMinTransactionTimeFromUnixTime(1000), minDbTransactionTime)
	assert.Equal(t, utils.GetMaxTransactionTimeFromUnixTime(5000), maxDbTransactionTime)
}

func TestComputeGoogleSheetDuplicateQueryBounds_CollectsDistinctSourceAndDestinationAccountIds(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		{Type: models.TRANSACTION_TYPE_TRANSFER, Time: 1000, SourceAccountId: 1, DestinationAccountId: 2},
		{Type: models.TRANSACTION_TYPE_EXPENSE, Time: 2000, SourceAccountId: 1, DestinationAccountId: 0},
		{Type: models.TRANSACTION_TYPE_EXPENSE, Time: 3000, SourceAccountId: 3, DestinationAccountId: 0},
	}

	_, _, accountIds := computeGoogleSheetDuplicateQueryBounds(items)

	assert.ElementsMatch(t, []int64{1, 2, 3}, accountIds)
}
