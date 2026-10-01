import type { PreviewRow } from '../types/domain';

class TableState {
  // Use $state.raw to avoid reactive proxying of large row arrays (thousands of cells)
  previewRows = $state.raw<PreviewRow[]>([]);
  activeSheetIndex = $state<number>(0);
  disabledColumnsBySheet = $state<Record<string, number[]>>({});
  isBefore = $state<boolean>(false);
  isFullscreen = $state<boolean>(false);
  page = $state<number>(1);
  pageSize = $state<number>(50);
  isLoadingRows = $state<boolean>(false);

  setPreviewRows(rows: PreviewRow[]): void {
    this.previewRows = rows;
  }

  appendPreviewRows(rows: PreviewRow[]): void {
    this.previewRows = [...this.previewRows, ...rows];
  }

  setActiveSheet(index: number): void {
    this.activeSheetIndex = index;
  }

  toggleColumn(sheetName: string, colIndex: number): void {
    const current = this.disabledColumnsBySheet[sheetName] || [];
    const next = current.includes(colIndex)
      ? current.filter((c) => c !== colIndex)
      : [...current, colIndex];

    this.disabledColumnsBySheet = {
      ...this.disabledColumnsBySheet,
      [sheetName]: next
    };
  }

  isColumnDisabled(sheetName: string, colIndex: number): boolean {
    const list = this.disabledColumnsBySheet[sheetName];
    return list ? list.includes(colIndex) : false;
  }

  setIsBefore(isBefore: boolean): void {
    this.isBefore = isBefore;
  }

  setIsFullscreen(isFullscreen: boolean): void {
    this.isFullscreen = isFullscreen;
  }

  setPage(page: number): void {
    this.page = page;
  }

  setPageSize(pageSize: number): void {
    this.pageSize = pageSize;
  }

  setIsLoadingRows(isLoading: boolean): void {
    this.isLoadingRows = isLoading;
  }

  reset(): void {
    this.previewRows = [];
    this.activeSheetIndex = 0;
    this.disabledColumnsBySheet = {};
    this.isBefore = false;
    this.isFullscreen = false;
    this.page = 1;
    this.pageSize = 50;
    this.isLoadingRows = false;
  }
}

export const tableState = new TableState();
