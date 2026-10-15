export const MIN_ZOOM = 0.70;
export const MAX_ZOOM = 2.00;
export const DEFAULT_ZOOM = 1.00;
export const BUTTON_STEP = 0.10;
export const WHEEL_STEP = 0.05;

/**
 * Ограничивает масштаб допустимым диапазоном [MIN_ZOOM, MAX_ZOOM]
 * и округляет значение до двух знаков после запятой для предотвращения погрешностей float.
 */
export function clampZoom(val: number): number {
  if (typeof val !== 'number' || isNaN(val)) return DEFAULT_ZOOM;
  const clamped = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, val));
  return Math.round(clamped * 100) / 100;
}

/**
 * Безопасно парсит сохранённый в localStorage масштаб.
 */
export function parseSavedZoom(saved: string | null | undefined): number {
  if (!saved) return DEFAULT_ZOOM;
  const parsed = parseFloat(saved);
  if (isNaN(parsed)) return DEFAULT_ZOOM;
  return clampZoom(parsed);
}
