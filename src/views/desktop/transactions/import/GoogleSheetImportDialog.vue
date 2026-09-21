<template>
    <v-dialog width="960" :persistent="!!persistent || fetching || submitting" v-model="showState">
        <one-column-dialog-layout :title="tt('Import from Google Sheets')" :cancel-button-title="tt('Cancel')"
                                  :disabled="fetching || submitting"
                                  :loading="fetching"
                                  content-class="pa-0"
                                  @cancel="close(currentStep === 'finalResult')">
            <template #after-title>
                <v-btn density="compact" color="default" variant="text" class="ms-2"
                       :aria-label="tt('Back')" :icon="true" :disabled="fetching"
                       @click="currentStep = 'enterUrl'"
                       v-if="currentStep === 'checkData'">
                    <v-icon :icon="mdiArrowLeft" size="22" />
                    <v-tooltip activator="parent">{{ tt('Back') }}</v-tooltip>
                </v-btn>
            </template>

            <template #toolbar>
                <v-btn class="ms-2 me-1" density="comfortable" variant="outlined" color="primary"
                       :disabled="fetching || !sheetUrl.trim()"
                       @click="fetchSheet"
                       v-if="currentStep === 'enterUrl'">
                    {{ tt('Fetch Sheet') }}
                    <v-progress-circular indeterminate size="22" class="ms-2" v-if="fetching"></v-progress-circular>
                </v-btn>

                <v-btn class="ms-2" density="comfortable" variant="outlined"
                       :disabled="submitting || readyToImportCount < 1"
                       @click="submit"
                       v-if="currentStep === 'checkData'">
                    {{ (submitting && importProcess > 0 ? tt('format.misc.importingTransactions', { process: formatNumberToLocalizedNumerals(importProcess, 2) }) : tt('Import')) }}
                    <v-progress-circular indeterminate size="22" class="ms-2" v-if="submitting"></v-progress-circular>
                </v-btn>
            </template>

            <template #content>
                <v-window class="disable-tab-transition" v-model="currentStep">
                    <v-window-item value="enterUrl">
                        <div class="pa-4">
                            <div class="d-flex align-center gap-2">
                                <v-text-field class="flex-grow-1"
                                    :label="tt('Google Sheets URL')"
                                    :placeholder="tt('e.g. https://docs.google.com/spreadsheets/d/...')"
                                    :disabled="fetching"
                                    v-model="sheetUrl"
                                    autofocus
                                    @keyup.enter="fetchSheet"
                                />
                                <v-btn density="comfortable" color="default" variant="text" class="mb-5"
                                       :aria-label="tt('Recently Used')" :icon="true"
                                       :disabled="fetching || sortedUrlHistory.length < 1">
                                    <v-icon :icon="mdiHistory" />
                                    <v-tooltip activator="parent">{{ tt('Recently Used') }}</v-tooltip>
                                    <v-menu activator="parent" location="bottom end" max-height="320" v-if="sortedUrlHistory.length > 0">
                                        <v-list density="compact">
                                            <v-list-item :key="entry.url"
                                                         :title="getHistoryEntryDisplayLabel(entry)"
                                                         :subtitle="getDisplayHistoryTime(entry.lastUsedTime)"
                                                         v-for="entry in sortedUrlHistory"
                                                         @click="selectHistoryUrl(entry.url)">
                                                <v-tooltip activator="parent" location="start">{{ entry.url }}</v-tooltip>
                                                <template #append>
                                                    <v-icon size="small" :icon="mdiClose"
                                                            :aria-label="tt('Remove')"
                                                            @click.stop="removeHistoryUrl(entry.url)" />
                                                </template>
                                            </v-list-item>
                                        </v-list>
                                    </v-menu>
                                </v-btn>
                            </div>
                            <v-alert type="info" variant="tonal" density="compact" class="mt-4">
                                {{ tt('Before pasting data, set the Time and Timezone columns to Plain Text (Format → Number → Plain text) — otherwise Google Sheets may silently reformat the dates and the import will fail.') }}
                            </v-alert>
                        </div>
                    </v-window-item>

                    <v-window-item value="checkData">
                        <div class="pa-4">
                            <v-alert type="info" variant="tonal" density="compact" class="mb-3">
                                {{ tt('format.misc.googleSheetRowsFound', { count: formatNumberToLocalizedNumerals(totalRowCount) }) }}
                                &middot;
                                {{ tt('format.misc.googleSheetRowsReadyToImport', { count: formatNumberToLocalizedNumerals(readyToImportCount) }) }}
                                <template v-if="duplicateRowCount > 0">
                                    &middot;
                                    {{ tt('format.misc.googleSheetRowsDuplicate', { count: formatNumberToLocalizedNumerals(duplicateRowCount) }) }}
                                </template>
                            </v-alert>
                            <v-alert type="warning" variant="tonal" density="compact" class="mb-3" v-if="invalidRowCount > 0">
                                {{ tt('Rows with an unresolved account or category cannot be imported. Fix the sheet and fetch it again.') }}
                            </v-alert>
                            <div class="d-flex align-center justify-space-between mb-2">
                                <div class="text-caption text-medium-emphasis">
                                    {{ tt('Use the checkbox in the header to select or unselect every row, or hold Shift while clicking a checkbox to select a range of rows.') }}
                                </div>
                                <v-checkbox density="compact" hide-details class="flex-grow-0 ms-4"
                                            :label="tt('format.misc.googleSheetRowsAlreadyImported', { count: formatNumberToLocalizedNumerals(alreadyImportedRowCount) })"
                                            v-model="showAlreadyImportedRows"
                                            v-if="alreadyImportedRowCount > 0" />
                            </div>
                            <v-table density="compact" fixed-header height="420" class="google-sheet-import-table">
                                <thead>
                                    <tr>
                                        <th style="width: 44px">
                                            <v-checkbox density="compact" color="primary"
                                                        :aria-label="allSelectableRowsSelected ? tt('Unselect All') : tt('Select All')"
                                                        :disabled="selectableRowCount < 1"
                                                        :model-value="allSelectableRowsSelected"
                                                        :indeterminate="someSelectableRowsSelected"
                                                        @update:model-value="setAllRowsSelection(!!$event)" />
                                            <v-tooltip activator="parent">{{ allSelectableRowsSelected ? tt('Unselect All') : tt('Select All') }}</v-tooltip>
                                        </th>
                                        <th>{{ tt('Row') }}</th>
                                        <th>{{ tt('Time') }}</th>
                                        <th>{{ tt('Type') }}</th>
                                        <th>{{ tt('Category') }}</th>
                                        <th>{{ tt('Account') }}</th>
                                        <th class="text-right">{{ tt('Amount') }}</th>
                                        <th>{{ tt('Description') }}</th>
                                        <th></th>
                                    </tr>
                                </thead>
                                <tbody>
                                    <tr :key="transaction.index" v-for="(transaction, rowIndex) in visibleTransactions">
                                        <td>
                                            <v-checkbox density="compact"
                                                        :color="!transaction.valid ? 'error' : 'primary'"
                                                        :disabled="!transaction.valid"
                                                        :model-value="transaction.selected"
                                                        @mousedown="captureRangeSelectionModifier"
                                                        @keydown="captureRangeSelectionModifier"
                                                        @update:model-value="setRowSelection(rowIndex, !!$event)" />
                                        </td>
                                        <td>{{ duplicateInfo[transaction.index]?.rowNumber }}</td>
                                        <td>{{ getDisplayDateTime(transaction) }}</td>
                                        <td>
                                            <v-chip label color="secondary" variant="outlined" size="x-small" v-if="transaction.type === TransactionType.ModifyBalance">{{ tt('Modify Balance') }}</v-chip>
                                            <v-chip label class="text-income" variant="outlined" size="x-small" v-else-if="transaction.type === TransactionType.Income">{{ tt('Income') }}</v-chip>
                                            <v-chip label class="text-expense" variant="outlined" size="x-small" v-else-if="transaction.type === TransactionType.Expense">{{ tt('Expense') }}</v-chip>
                                            <v-chip label color="primary" variant="outlined" size="x-small" v-else-if="transaction.type === TransactionType.Transfer">{{ tt('Transfer') }}</v-chip>
                                        </td>
                                        <td>{{ transaction.actualCategoryName || transaction.originalCategoryName }}</td>
                                        <td>{{ transaction.actualSourceAccountName || transaction.originalSourceAccountName }}</td>
                                        <td class="text-right">{{ getDisplayAmount(transaction) }}</td>
                                        <td>{{ transaction.comment }}</td>
                                        <td>
                                            <template v-if="duplicateInfo[transaction.index]?.alreadyImported">
                                                <v-chip size="x-small" color="info" variant="flat">{{ tt('Already Imported') }}</v-chip>
                                                <v-tooltip activator="parent">{{ tt('This row was already imported from this sheet') }}</v-tooltip>
                                            </template>
                                            <template v-else-if="duplicateInfo[transaction.index]?.isDuplicate">
                                                <v-chip size="x-small" color="warning" variant="flat">{{ tt('Duplicate') }}</v-chip>
                                                <v-tooltip activator="parent">{{ duplicateInfo[transaction.index]?.duplicateReason === 'existing_transaction' ? tt('Duplicate of an existing transaction') : tt('Duplicate row already in this sheet') }}</v-tooltip>
                                            </template>
                                            <v-icon v-else-if="!transaction.valid" :icon="mdiAlertCircleOutline" color="error" size="small">
                                                <v-tooltip activator="parent">{{ tt('Rows with an unresolved account or category cannot be imported. Fix the sheet and fetch it again.') }}</v-tooltip>
                                            </v-icon>
                                        </td>
                                    </tr>
                                </tbody>
                            </v-table>
                        </div>
                    </v-window-item>

                    <v-window-item value="finalResult">
                        <div class="mx-4 my-4">
                            <v-alert type="success" color="success-darken-1" variant="tonal">{{ tt('Data Import Completed') }}</v-alert>
                            <div class="text-body-large my-4">{{ tt('format.misc.importTransactionResult', { count: formatNumberToLocalizedNumerals(importedCount || 0) }) }}</div>
                        </div>
                    </v-window-item>
                </v-window>
            </template>
        </one-column-dialog-layout>
    </v-dialog>

    <confirm-dialog ref="confirmDialog"/>
    <snack-bar ref="snackbar" />
</template>

<script setup lang="ts">
import { ref, computed, useTemplateRef } from 'vue';

import { useI18n } from '@/locales/helpers.ts';

import { useAccountsStore } from '@/stores/account.ts';
import { useTransactionsStore } from '@/stores/transaction.ts';
import { useOverviewStore } from '@/stores/overview.ts';
import { useStatisticsStore } from '@/stores/statistics.ts';
import { useSettingsStore } from '@/stores/setting.ts';

import { TransactionType } from '@/core/transaction.ts';
import type { GoogleSheetImportUrlHistoryEntry } from '@/core/setting.ts';
import type { GoogleSheetImportRowKey } from '@/models/google_sheet_import.ts';
import { ImportTransaction } from '@/models/imported_transaction.ts';

import { isNumber } from '@/lib/common.ts';
import { parseBigDecimal } from '@/lib/numeral.ts';
import { parseDateTimeFromUnixTimeWithTimezoneOffset, parseDateTimeFromUnixTime } from '@/lib/datetime.ts';
import { generateRandomUUID } from '@/lib/misc.ts';

import OneColumnDialogLayout from '@/components/desktop/OneColumnDialogLayout.vue';
import ConfirmDialog from '@/components/desktop/ConfirmDialog.vue';
import SnackBar from '@/components/desktop/SnackBar.vue';

import {
    mdiArrowLeft,
    mdiAlertCircleOutline,
    mdiHistory,
    mdiClose
} from '@mdi/js';

type ConfirmDialogType = InstanceType<typeof ConfirmDialog>;
type SnackBarType = InstanceType<typeof SnackBar>;

type GoogleSheetImportDialogStep = 'enterUrl' | 'checkData' | 'finalResult';

interface GoogleSheetDuplicateInfo {
    rowNumber: number;
    rowHash: string;
    occurrence: number;
    isDuplicate: boolean;
    alreadyImported: boolean;
    duplicateReason?: string;
}

defineProps<{
    persistent?: boolean;
}>();

const {
    tt,
    formatNumberToLocalizedNumerals,
    formatDateTimeToLongDateTime,
    formatAmountToLocalizedNumeralsWithCurrency
} = useI18n();

const accountsStore = useAccountsStore();
const transactionsStore = useTransactionsStore();
const overviewStore = useOverviewStore();
const statisticsStore = useStatisticsStore();
const settingsStore = useSettingsStore();

const confirmDialog = useTemplateRef<ConfirmDialogType>('confirmDialog');
const snackbar = useTemplateRef<SnackBarType>('snackbar');

let resolveFunc: (() => void) | null = null;
let rejectFunc: ((reason?: unknown) => void) | null = null;

const showState = ref<boolean>(false);
const currentStep = ref<GoogleSheetImportDialogStep>('enterUrl');
const fetching = ref<boolean>(false);
const submitting = ref<boolean>(false);
const importProcess = ref<number>(0);
const sheetUrl = ref<string>('');
const clientSessionId = ref<string>('');
const importTransactions = ref<ImportTransaction[]>([]);
const duplicateInfo = ref<Record<number, GoogleSheetDuplicateInfo>>({});
const totalRowCount = ref<number>(0);
const duplicateRowCount = ref<number>(0);
const alreadyImportedRowCount = ref<number>(0);
const showAlreadyImportedRows = ref<boolean>(false);
const spreadsheetId = ref<string>('');
const sheetGid = ref<string>('');
const importedCount = ref<number | undefined>(undefined);

// Shift-click range selection needs the modifier state from the pointer/key event, because
// v-checkbox's update:model-value only carries the new value. Both are plain (non-reactive)
// variables on purpose - nothing in the template renders them.
let rangeSelectionModifierPressed: boolean = false;
let lastToggledRowIndex: number | null = null;

const readyToImportCount = computed<number>(() => importTransactions.value.filter(transaction => transaction.valid && transaction.selected).length);
const invalidRowCount = computed<number>(() => importTransactions.value.filter(transaction => !transaction.valid).length);

// Rows that were already imported from this sheet are hidden by default - they are already in the
// books, so re-listing them every time is just noise. They stay one checkbox away for the case where
// the user genuinely wants to import the same row again.
const visibleTransactions = computed<ImportTransaction[]>(() => {
    if (showAlreadyImportedRows.value) {
        return importTransactions.value;
    }

    return importTransactions.value.filter(transaction => !duplicateInfo.value[transaction.index]?.alreadyImported);
});

// Select all and shift-range act on what the user can actually see, so hidden rows are never
// selected behind their back.
const selectableVisibleTransactions = computed<ImportTransaction[]>(() => visibleTransactions.value.filter(transaction => transaction.valid));
const selectedVisibleRowCount = computed<number>(() => selectableVisibleTransactions.value.filter(transaction => transaction.selected).length);
const selectableRowCount = computed<number>(() => selectableVisibleTransactions.value.length);
const allSelectableRowsSelected = computed<boolean>(() => selectableRowCount.value > 0 && selectedVisibleRowCount.value === selectableRowCount.value);
const someSelectableRowsSelected = computed<boolean>(() => selectedVisibleRowCount.value > 0 && selectedVisibleRowCount.value < selectableRowCount.value);

const sortedUrlHistory = computed<GoogleSheetImportUrlHistoryEntry[]>(() => {
    return settingsStore.getGoogleSheetImportUrlHistory()
        .slice()
        .sort((entry1, entry2) => entry2.lastUsedTime - entry1.lastUsedTime);
});

function getHistoryEntryDisplayLabel(entry: GoogleSheetImportUrlHistoryEntry): string {
    return entry.displayName || entry.url;
}

function selectHistoryUrl(url: string): void {
    sheetUrl.value = url;
}

function removeHistoryUrl(url: string): void {
    settingsStore.removeGoogleSheetImportUrlFromHistory(url);
}

function getDisplayHistoryTime(unixTime: number): string {
    return formatDateTimeToLongDateTime(parseDateTimeFromUnixTime(unixTime));
}

// Google's export response only sometimes includes usable spreadsheet/sheet names (best-effort,
// undocumented header) - build "Spreadsheet(Sheet)" when both are known, fall back to whichever one
// is available, and leave it undefined (so the raw url is shown instead) when neither is.
function getGoogleSheetDisplayName(spreadsheetName?: string, sheetName?: string): string | undefined {
    if (spreadsheetName && sheetName) {
        return `${spreadsheetName}(${sheetName})`;
    }

    return spreadsheetName || sheetName || undefined;
}

function open(): Promise<void> {
    currentStep.value = 'enterUrl';
    fetching.value = false;
    submitting.value = false;
    importProcess.value = 0;
    sheetUrl.value = sortedUrlHistory.value[0]?.url ?? '';
    importTransactions.value = [];
    duplicateInfo.value = {};
    totalRowCount.value = 0;
    duplicateRowCount.value = 0;
    alreadyImportedRowCount.value = 0;
    showAlreadyImportedRows.value = false;
    spreadsheetId.value = '';
    sheetGid.value = '';
    importedCount.value = undefined;
    clientSessionId.value = generateRandomUUID();
    rangeSelectionModifierPressed = false;
    lastToggledRowIndex = null;
    showState.value = true;

    return new Promise((resolve, reject) => {
        resolveFunc = resolve;
        rejectFunc = reject;
    });
}

function fetchSheet(): void {
    const trimmedUrl = sheetUrl.value.trim();

    if (!trimmedUrl) {
        snackbar.value?.showError('Please enter a Google Sheets URL');
        return;
    }

    fetching.value = true;

    transactionsStore.parseGoogleSheetImport({ url: trimmedUrl }).then(response => {
        fetching.value = false;

        const transactions: ImportTransaction[] = [];
        const info: Record<number, GoogleSheetDuplicateInfo> = {};

        for (const item of response.items) {
            const transaction = ImportTransaction.of(item, transactions.length);
            transaction.selected = transaction.valid && !item.isDuplicate;
            transactions.push(transaction);
            info[transaction.index] = {
                rowNumber: item.rowNumber,
                rowHash: item.rowHash,
                occurrence: item.occurrence,
                isDuplicate: item.isDuplicate,
                alreadyImported: item.alreadyImported,
                duplicateReason: item.duplicateReason
            };
        }

        importTransactions.value = transactions;
        duplicateInfo.value = info;
        rangeSelectionModifierPressed = false;
        lastToggledRowIndex = null;
        totalRowCount.value = response.totalRowCount;
        duplicateRowCount.value = response.duplicateRowCount;
        alreadyImportedRowCount.value = response.alreadyImportedRowCount;
        showAlreadyImportedRows.value = false;
        spreadsheetId.value = response.spreadsheetId;
        sheetGid.value = response.gid;
        currentStep.value = 'checkData';
        settingsStore.addGoogleSheetImportUrlToHistory(trimmedUrl, getGoogleSheetDisplayName(response.spreadsheetName, response.sheetName));
    }).catch(error => {
        fetching.value = false;

        if (!error.processed) {
            snackbar.value?.showError(error);
        }
    });
}

function captureRangeSelectionModifier(event: MouseEvent | KeyboardEvent): void {
    rangeSelectionModifierPressed = event.shiftKey;
}

function setRowSelection(rowIndex: number, selected: boolean): void {
    let startIndex = rowIndex;
    let endIndex = rowIndex;

    if (rangeSelectionModifierPressed && lastToggledRowIndex !== null) {
        startIndex = Math.min(lastToggledRowIndex, rowIndex);
        endIndex = Math.max(lastToggledRowIndex, rowIndex);
    }

    for (let i = startIndex; i <= endIndex; i++) {
        const transaction = visibleTransactions.value[i];

        // invalid rows can never be imported, so they stay unselected even inside a range
        if (transaction && transaction.valid) {
            transaction.selected = selected;
        }
    }

    rangeSelectionModifierPressed = false;
    lastToggledRowIndex = rowIndex;
}

function setAllRowsSelection(selected: boolean): void {
    for (const transaction of selectableVisibleTransactions.value) {
        transaction.selected = selected;
    }

    rangeSelectionModifierPressed = false;
    lastToggledRowIndex = null;
}

function getDisplayDateTime(transaction: ImportTransaction): string {
    return formatDateTimeToLongDateTime(parseDateTimeFromUnixTimeWithTimezoneOffset(transaction.time, transaction.utcOffset));
}

function getDisplayAmount(transaction: ImportTransaction): string {
    return formatAmountToLocalizedNumeralsWithCurrency(parseBigDecimal(transaction.sourceAmount), transaction.originalSourceAccountCurrency);
}

function submit(): void {
    const transactions: ImportTransaction[] = [];
    // rowKeys is positional against transactions, so the server can record which sheet row created
    // each transaction and recognize it on a later import of the same sheet
    const rowKeys: GoogleSheetImportRowKey[] = [];

    for (const transaction of importTransactions.value) {
        if (transaction.valid && transaction.selected) {
            const info = duplicateInfo.value[transaction.index];
            transactions.push(transaction);
            rowKeys.push({
                rowHash: info?.rowHash ?? '',
                occurrence: info?.occurrence ?? 0
            });
        }
    }

    if (transactions.length < 1) {
        snackbar.value?.showError('No data to import');
        return;
    }

    confirmDialog.value?.open('format.misc.confirmImportTransactions', {
        count: formatNumberToLocalizedNumerals(transactions.length)
    }).then(() => {
        submitting.value = true;

        let showProcessTimer: number | undefined = undefined;

        if (transactions.length > 100) {
            setTimeout(() => {
                if (!submitting.value) {
                    return;
                }

                // @ts-expect-error the return value of setInterval is number, but lint shows it as NodeJS.Timer
                showProcessTimer = setInterval(() => {
                    if (submitting.value) {
                        transactionsStore.getImportTransactionsProcess({
                            clientSessionId: clientSessionId.value
                        }).then(response => {
                            if (isNumber(response) && 0 <= response && response < 100) {
                                importProcess.value = response;
                            } else {
                                importProcess.value = 0;
                                clearInterval(showProcessTimer);
                                showProcessTimer = undefined;
                            }
                        }).catch(() => {
                            importProcess.value = 0;
                            clearInterval(showProcessTimer);
                            showProcessTimer = undefined;
                        });
                    }
                }, 2000);
            }, 2000);
        }

        transactionsStore.importTransactions({
            transactions: transactions,
            clientSessionId: clientSessionId.value,
            googleSheetImport: spreadsheetId.value ? {
                spreadsheetId: spreadsheetId.value,
                gid: sheetGid.value,
                rowKeys: rowKeys
            } : undefined
        }).then(response => {
            if (showProcessTimer) {
                importProcess.value = 0;
                clearInterval(showProcessTimer);
                showProcessTimer = undefined;
            }

            importedCount.value = response;
            currentStep.value = 'finalResult';

            accountsStore.updateAccountListInvalidState(true);
            transactionsStore.updateTransactionListInvalidState(true);
            overviewStore.updateTransactionOverviewInvalidState(true);
            statisticsStore.updateTransactionStatisticsInvalidState(true);

            submitting.value = false;
        }).catch(error => {
            if (showProcessTimer) {
                importProcess.value = 0;
                clearInterval(showProcessTimer);
                showProcessTimer = undefined;
            }

            submitting.value = false;

            if (!error.processed) {
                snackbar.value?.showError(error);
            }
        });
    });
}

function close(completed: boolean): void {
    if (completed) {
        resolveFunc?.();
    } else {
        rejectFunc?.();
    }

    showState.value = false;
}

defineExpose({
    open
});
</script>

<style scoped>
.google-sheet-import-table :deep(td),
.google-sheet-import-table :deep(th) {
    white-space: nowrap;
}
</style>
