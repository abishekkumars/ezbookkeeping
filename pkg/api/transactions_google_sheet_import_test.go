package api

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestMarkInSheetDuplicates_NoDuplicates(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(2000, 1, 1, 200, "dinner"),
		newTestGoogleSheetImportItem(3000, 2, 1, 300, "groceries"),
	}

	actualValue := markInSheetDuplicates(items)

	assert.Empty(t, actualValue)
}

func TestMarkInSheetDuplicates_ExactDuplicateRow(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
	}

	actualValue := markInSheetDuplicates(items)

	assert.Len(t, actualValue, 1)
	assert.Equal(t, models.GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET, actualValue[1])
	_, firstRowFlagged := actualValue[0]
	assert.False(t, firstRowFlagged)
}

func TestMarkInSheetDuplicates_DifferentAmountIsNotADuplicate(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 150, "lunch"),
	}

	actualValue := markInSheetDuplicates(items)

	assert.Empty(t, actualValue)
}

func TestMarkInSheetDuplicates_DifferentCommentIsNotADuplicate(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "dinner"),
	}

	actualValue := markInSheetDuplicates(items)

	assert.Empty(t, actualValue)
}

func TestMarkInSheetDuplicates_MultipleDuplicatesOfSameRow(t *testing.T) {
	items := []*models.ImportTransactionResponse{
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(2000, 1, 1, 200, "dinner"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
		newTestGoogleSheetImportItem(1000, 1, 1, 100, "lunch"),
	}

	actualValue := markInSheetDuplicates(items)

	assert.Len(t, actualValue, 2)
	assert.Equal(t, models.GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET, actualValue[2])
	assert.Equal(t, models.GOOGLE_SHEET_DUPLICATE_REASON_IN_SHEET, actualValue[3])
}

func TestMarkInSheetDuplicates_EmptyItems(t *testing.T) {
	actualValue := markInSheetDuplicates([]*models.ImportTransactionResponse{})

	assert.Empty(t, actualValue)
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
