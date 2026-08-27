import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import { readFile } from 'node:fs/promises';

const require = createRequire(new URL('../web/package.json', import.meta.url));
const { JSDOM } = require('jsdom');
const source = process.env.CODE_TOUR_BASELINE_COMMIT ? execFileSync('git', ['show', `${process.env.CODE_TOUR_BASELINE_COMMIT}:docs/code-tour.html`], { encoding: 'utf8' }) : await readFile(new URL('./code-tour.html', import.meta.url), 'utf8');

test('answers survive recreation and safe failure modes', () => {
  const values = new Map();
  const make = (hash = '', storage = values) => {
    const dom = new JSDOM(source, { url: `file:///tour/code-tour.html${hash}`, runScripts: 'dangerously', beforeParse(window) {
      Object.defineProperty(window, 'localStorage', { configurable: true, value: { getItem: key => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value) } });
      Object.defineProperty(window.history, 'scrollRestoration', { configurable: true, writable: true, value: 'auto' });
      const frames = [];
      window.requestAnimationFrame = callback => frames.push(callback);
      window.drainFrame = () => frames.shift()?.();
      window.IntersectionObserver = class { constructor(callback, options) { window.observerOptions = options; window.emitVisible = node => callback([{ isIntersecting: true, target: node }]); } observe() { window.observed = true; } disconnect() {} };
      window.HTMLElement.prototype.scrollIntoView = function (options) { window.scrolled = [this.id, options]; };
    }});
    return dom;
  };
  const first = make();
  assert.equal(first.window.history.scrollRestoration, 'manual');
  const answer = first.window.document.querySelector('#unit-7 textarea');
  assert.ok(answer, 'answer field is absent');
  answer.value = 'because the agent pulls desired state';
  answer.dispatchEvent(new first.window.Event('input', { bubbles: true }));
  assert.match(first.window.document.querySelector('#progress-status').textContent, /^1 of 15 questions answered/);
  first.window.dispatchEvent(new first.window.Event('load')); first.window.drainFrame(); first.window.drainFrame();
  first.window.emitVisible?.(first.window.document.querySelector('#unit-7'));
  const second = make(); second.window.dispatchEvent(new second.window.Event('load'));
  assert.equal(second.window.scrolled, undefined); assert.equal(second.window.observed, undefined); second.window.drainFrame();
  assert.equal(second.window.scrolled[0], 'unit-7'); assert.equal(second.window.scrolled[1].block, 'center'); assert.equal(second.window.scrolled[1].behavior, 'instant');
  assert.equal(second.window.observed, undefined); second.window.drainFrame(); assert.equal(second.window.observed, true); assert.equal(second.window.observerOptions.rootMargin, '-45% 0px -45% 0px');
  assert.equal(second.window.document.querySelector('#unit-7 textarea').value, 'because the agent pulls desired state');
  const withHash = make('#unit-3'); withHash.window.dispatchEvent(new withHash.window.Event('load')); withHash.window.drainFrame(); assert.equal(withHash.window.scrolled, undefined); withHash.window.drainFrame();
  for (const stored of ['{broken', '{"answers":null}', '{"answers":[]}', '{"answers":{"unit-7":7}}']) {
    const malformed = make('', new Map([['cadestro.code-tour.v2', stored]])); malformed.window.dispatchEvent(new malformed.window.Event('load')); malformed.window.drainFrame(); malformed.window.drainFrame();
    const status = malformed.window.document.querySelector('#progress-status').textContent; assert.match(status, /^0 of 15 questions answered/); assert.match(status, /saved automatically in this browser/); assert.doesNotMatch(status, /storage is unavailable/); malformed.window.close();
  }
  const unavailableGet = make('', { getItem() { throw new Error('blocked'); }, setItem() { throw new Error('blocked'); } }); assert.match(unavailableGet.window.document.querySelector('#progress-status').textContent, /not saved because browser storage is unavailable/); unavailableGet.window.close();
  const unavailableSet = make('', { getItem() { return null; }, setItem() { throw new Error('blocked'); } }); const unavailableAnswer = unavailableSet.window.document.querySelector('#unit-1 textarea'); unavailableAnswer.value = 'draft'; unavailableAnswer.dispatchEvent(new unavailableSet.window.Event('input', { bubbles: true })); assert.match(unavailableSet.window.document.querySelector('#progress-status').textContent, /not saved because browser storage is unavailable/); unavailableSet.window.close();
  for (const dom of [first, second, withHash]) dom.window.close();
});
