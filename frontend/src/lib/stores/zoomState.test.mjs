import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  MIN_ZOOM,
  MAX_ZOOM,
  DEFAULT_ZOOM,
  BUTTON_STEP,
  WHEEL_STEP,
  clampZoom,
  parseSavedZoom,
} from './zoomLogic.ts';

describe('zoomState core logic', () => {
  it('defines correct zoom bounds and default constants', () => {
    assert.equal(MIN_ZOOM, 0.70);
    assert.equal(MAX_ZOOM, 2.00);
    assert.equal(DEFAULT_ZOOM, 1.00);
    assert.equal(BUTTON_STEP, 0.10);
    assert.equal(WHEEL_STEP, 0.05);
  });

  it('clamps values within [0.70, 2.00] and rounds floating point issues', () => {
    assert.equal(clampZoom(1.0), 1.0);
    assert.equal(clampZoom(0.5), 0.70);
    assert.equal(clampZoom(2.5), 2.00);
    assert.equal(clampZoom(1.0500000000000003), 1.05);
    assert.equal(clampZoom(NaN), 1.00);
  });

  it('safely parses stored localStorage values', () => {
    assert.equal(parseSavedZoom(null), 1.00);
    assert.equal(parseSavedZoom(''), 1.00);
    assert.equal(parseSavedZoom('invalid'), 1.00);
    assert.equal(parseSavedZoom('1.25'), 1.25);
    assert.equal(parseSavedZoom('0.1'), 0.70);
    assert.equal(parseSavedZoom('5.0'), 2.00);
  });
});
