import type { ImportTransactionResponse } from './imported_transaction.ts';

export interface GoogleSheetImportPreviewItemResponse extends ImportTransactionResponse {
    readonly rowNumber: number;
    readonly rowHash: string;
    readonly isDuplicate: boolean;
    readonly alreadyImported: boolean;
    readonly duplicateReason?: string;
}

export interface GoogleSheetImportPreviewResponse {
    readonly spreadsheetName?: string;
    readonly sheetName?: string;
    readonly spreadsheetId: string;
    readonly gid: string;
    readonly totalRowCount: number;
    readonly duplicateRowCount: number;
    readonly alreadyImportedRowCount: number;
    readonly items: GoogleSheetImportPreviewItemResponse[];
}

// GoogleSheetImportRowKey identifies one sheet row by its content fingerprint, so the server can
// record that it was imported. The row's position among identical rows is assigned server-side.
export interface GoogleSheetImportRowKey {
    readonly rowHash: string;
}

// GoogleSheetImportSource is sent with an import confirmation so each created transaction can be
// recorded against the sheet row it came from. rowKeys is positional against the submitted transactions.
export interface GoogleSheetImportSource {
    readonly spreadsheetId: string;
    readonly gid: string;
    readonly rowKeys: GoogleSheetImportRowKey[];
}
