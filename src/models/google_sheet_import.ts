import type { ImportTransactionResponse } from './imported_transaction.ts';

export interface GoogleSheetImportPreviewItemResponse extends ImportTransactionResponse {
    readonly rowNumber: number;
    readonly isDuplicate: boolean;
    readonly duplicateReason?: string;
}

export interface GoogleSheetImportPreviewResponse {
    readonly spreadsheetName?: string;
    readonly sheetName?: string;
    readonly totalRowCount: number;
    readonly duplicateRowCount: number;
    readonly items: GoogleSheetImportPreviewItemResponse[];
}
