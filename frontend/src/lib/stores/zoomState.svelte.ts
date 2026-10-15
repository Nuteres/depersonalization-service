import {
  MIN_ZOOM,
  MAX_ZOOM,
  DEFAULT_ZOOM,
  BUTTON_STEP,
  WHEEL_STEP,
  clampZoom,
  parseSavedZoom,
} from './zoomLogic';

export { MIN_ZOOM, MAX_ZOOM, DEFAULT_ZOOM, BUTTON_STEP, WHEEL_STEP, clampZoom, parseSavedZoom };

export class ZoomState {
  currentZoom = $state<number>(DEFAULT_ZOOM);
  showIndicator = $state<boolean>(false);
  private indicatorTimeout: ReturnType<typeof setTimeout> | null = null;

  readonly minZoom = MIN_ZOOM;
  readonly maxZoom = MAX_ZOOM;
  readonly stepButtons = BUTTON_STEP;
  readonly stepWheel = WHEEL_STEP;

  init(): void {
    if (typeof window === 'undefined') return;
    try {
      const saved = localStorage.getItem('dep_app_zoom');
      this.currentZoom = parseSavedZoom(saved);
    } catch {
      this.currentZoom = DEFAULT_ZOOM;
    }
    this.applyZoom();
  }

  zoomIn(step: number = BUTTON_STEP): void {
    this.setZoom(this.currentZoom + step);
  }

  zoomOut(step: number = BUTTON_STEP): void {
    this.setZoom(this.currentZoom - step);
  }

  resetZoom(): void {
    this.setZoom(DEFAULT_ZOOM);
  }

  setZoom(val: number, showFeedback: boolean = false): void {
    this.currentZoom = clampZoom(val);
    this.applyZoom();
    this.saveZoom();
    if (showFeedback) {
      this.triggerIndicator();
    }
  }

  triggerIndicator(): void {
    this.showIndicator = true;
    if (this.indicatorTimeout) {
      clearTimeout(this.indicatorTimeout);
    }
    if (typeof window !== 'undefined') {
      this.indicatorTimeout = setTimeout(() => {
        this.showIndicator = false;
        this.indicatorTimeout = null;
      }, 1200);
    }
  }

  private applyZoom(): void {
    if (typeof document === 'undefined') return;
    // Apply CSS zoom to document root element
    document.documentElement.style.zoom = String(this.currentZoom);
  }

  private saveZoom(): void {
    if (typeof window === 'undefined') return;
    try {
      localStorage.setItem('dep_app_zoom', String(this.currentZoom));
    } catch (e) {
      console.warn('Failed to save zoom to localStorage:', e);
    }
  }
}

export const zoomState = new ZoomState();
