/**
 * Utility functions for cross-platform file path manipulation (Windows and POSIX).
 */

export interface PathParts {
  dir: string;
  filename: string;
  baseName: string;
  ext: string;
  sep: string;
}

/**
 * Splits a file path into directory, filename, baseName (without ext), extension, and directory separator.
 * Correctly handles both Windows backslashes (\) and POSIX forward slashes (/).
 */
export function splitPath(filePath: string): PathParts {
  const trimmed = filePath.trim();
  if (!trimmed) {
    return { dir: '', filename: '', baseName: '', ext: '', sep: '/' };
  }

  const lastSlash = trimmed.lastIndexOf('/');
  const lastBackslash = trimmed.lastIndexOf('\\');
  const sep = lastBackslash > lastSlash ? '\\' : '/';
  const lastSep = Math.max(lastSlash, lastBackslash);

  let dir = '';
  let filename = trimmed;
  if (lastSep !== -1) {
    dir = trimmed.slice(0, lastSep);
    filename = trimmed.slice(lastSep + 1);
  }

  const dotIndex = filename.lastIndexOf('.');
  const baseName = dotIndex > 0 ? filename.slice(0, dotIndex) : filename;
  const ext = dotIndex > 0 ? filename.slice(dotIndex + 1).toLowerCase() : '';

  return { dir, filename, baseName, ext, sep };
}

/**
 * Generates the default output path in the same directory as sourceFilePath with the given target extension.
 * If baseName already ends with the suffix, avoids duplicate suffixes.
 */
export function getDefaultOutputPath(
  sourceFilePath: string,
  targetExt: string,
  suffix = '_anonymized'
): string {
  if (!sourceFilePath) return '';
  const { dir, baseName, sep } = splitPath(sourceFilePath);
  const cleanExt = targetExt.startsWith('.') ? targetExt.slice(1) : targetExt;

  let effectiveBase = baseName;
  if (suffix && !effectiveBase.endsWith(suffix)) {
    effectiveBase = `${effectiveBase}${suffix}`;
  }

  const newFilename = `${effectiveBase}.${cleanExt}`;
  return dir ? `${dir}${sep}${newFilename}` : newFilename;
}

/**
 * Generates the default restored file path in the same directory as dataFilePath.
 * Automatically removes any `_anonymized` suffix and appends `_restored`.
 */
export function getDefaultRestoredPath(
  dataFilePath: string,
  targetExt?: string
): string {
  if (!dataFilePath) return '';
  const { dir, baseName, ext, sep } = splitPath(dataFilePath);
  const cleanExt = (targetExt || ext || 'xlsx').replace(/^\./, '');

  let effectiveBase = baseName;
  if (effectiveBase.endsWith('_anonymized')) {
    effectiveBase = effectiveBase.slice(0, -'_anonymized'.length);
  }
  effectiveBase = `${effectiveBase}_restored`;

  const newFilename = `${effectiveBase}.${cleanExt}`;
  return dir ? `${dir}${sep}${newFilename}` : newFilename;
}
