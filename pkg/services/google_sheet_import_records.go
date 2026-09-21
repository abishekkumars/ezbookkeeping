package services

import (
	"xorm.io/xorm"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/datastore"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
)

// GoogleSheetImportRecordService represents the Google Sheet import record service
type GoogleSheetImportRecordService struct {
	ServiceUsingDB
}

// Initialize a Google Sheet import record service singleton instance
var (
	GoogleSheetImportRecords = &GoogleSheetImportRecordService{
		ServiceUsingDB: ServiceUsingDB{
			container: datastore.Container,
		},
	}
)

// GetImportRecordsBySheet returns all import records of the specified sheet tab for the specified user
func (s *GoogleSheetImportRecordService) GetImportRecordsBySheet(c core.Context, uid int64, spreadsheetId string, gid string) ([]*models.GoogleSheetImportRecord, error) {
	if uid <= 0 {
		return nil, errs.ErrUserIdInvalid
	}

	if spreadsheetId == "" {
		return nil, errs.ErrGoogleSheetUrlInvalid
	}

	var records []*models.GoogleSheetImportRecord
	err := s.UserDataDB(uid).NewSession(c).Where("uid=? AND spreadsheet_id=? AND gid=?", uid, spreadsheetId, gid).Find(&records)

	if err != nil {
		return nil, err
	}

	return records, nil
}

// BatchCreateImportRecordsInSession writes the specified import records within an existing database
// session, so they commit or roll back together with the transactions they describe.
//
// The unique index on (uid, spreadsheet_id, gid, row_hash, occurrence) is the backstop against
// importing the same sheet row twice: if two concurrent imports race past the preview check, the
// second insert fails here and rolls the whole import back rather than creating duplicates.
func (s *GoogleSheetImportRecordService) BatchCreateImportRecordsInSession(sess *xorm.Session, records []*models.GoogleSheetImportRecord) error {
	if len(records) < 1 {
		return nil
	}

	for i := 0; i < len(records); i++ {
		_, err := sess.Insert(records[i])

		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteImportRecordsByTransactionIds removes import records that point at the specified transactions
func (s *GoogleSheetImportRecordService) DeleteImportRecordsByTransactionIds(c core.Context, uid int64, transactionIds []int64) error {
	if uid <= 0 {
		return errs.ErrUserIdInvalid
	}

	if len(transactionIds) < 1 {
		return nil
	}

	return s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		_, err := sess.Where("uid=?", uid).In("transaction_id", transactionIds).Delete(&models.GoogleSheetImportRecord{})
		return err
	})
}
