import * as pdfjsLib from 'pdfjs-dist';
import pdfWorker from 'pdfjs-dist/build/pdf.worker.min.mjs?url';

// Configure the worker to use the local bundled asset (100% Air-Gapped)
pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorker;

export type { PDFDocumentProxy, PDFPageProxy } from 'pdfjs-dist';
export { pdfjsLib };

/**
 * Loads a PDF document from raw binary data (Uint8Array or ArrayBuffer).
 */
export async function loadPdf(data: Uint8Array | ArrayBuffer): Promise<pdfjsLib.PDFDocumentProxy> {
  const loadingTask = pdfjsLib.getDocument({
    data,
    cMapUrl: undefined, // Default internal CMaps
    cMapPacked: true,
    isEvalSupported: false,
  });
  return loadingTask.promise;
}
