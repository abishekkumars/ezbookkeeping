package api

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/models"
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
