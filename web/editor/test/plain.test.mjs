// SPDX-License-Identifier: MIT

// A password typed as text (ADR-7, 2026-11-20): what the editor asks about before it saves, and what it leaves alone.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { findPlain, isLabel } from '../src/plain.js';

const values = (md) => findPlain(md).filter((f) => !f.partial).map((f) => f.value);

test('a label, a colon or an equals sign, and one word', () => {
  for (const [md, want] of [
    ['Router password: hunter2', 'hunter2'],
    ['**Password:** hunter2!', 'hunter2!'],
    ['- WLAN-Passwort = Sommer2026', 'Sommer2026'],
    ['> Kennwort: Tür#42a', 'Tür#42a'],
    ['PIN: 4711', '4711'],
    ['API key: sk_live_abcdef', 'sk_live_abcdef'],
    ['Adminpasswort: x9!kLm', 'x9!kLm'],
    ['Password: `Q8z!r2`', 'Q8z!r2'],
    ['Token = **abcd1234**', 'abcd1234'],
  ]) {
    assert.deepEqual(values(md), [want], md);
  }
});

test('the positions name the value in the text', () => {
  const md = 'Intro\n\nRouter password: hunter2\nend';
  const [f] = findPlain(md);
  assert.equal(md.slice(f.from, f.to), 'hunter2');
});

test('a row of a table', () => {
  const md = '| Device | Password |\n|---|---|\n| Router | x |\n\n| Password | hunter2 |\n| User | anna |';
  assert.deepEqual(values(md), ['hunter2']);
});

test('nothing in code or comments', () => {
  for (const md of [
    '```ini\npassword = hunter2\n```',
    '~~~\nPassword: hunter2\n~~~',
    '    password: hunter2',
    '\tpassword: hunter2',
    'Run `mysql --password=hunter2` once.',
    '`password: hunter2`',
    '<!-- password: hunter2 -->',
    '<!--\npassword: hunter2\n-->',
    '```\n```not closed\npassword: hunter2\n```',
  ]) {
    assert.deepEqual(values(md), [], md);
  }
});

test('placeholders, sentences, references and links are not values', () => {
  for (const md of [
    'Password: ***',
    'Password: …',
    'Password: xxxx',
    'Password: <password>',
    'Password: {{password}}',
    'Password: n/a',
    'Password: none',
    'Password: ask Bob',
    'Password: siehe unten',
    'Password: ::secret{id="7k2m9q4x1c"}',
    'Password: https://vault.example.org/item',
    'Password: [the vault](https://example.org)',
    'Password: abc',
    'Boarding pass: A1234',
    'Time: 12:30',
    'The password: of the router is long',
    'Passwords are kept in the vault: never here',
  ]) {
    assert.deepEqual(values(md), [], md);
  }
});

test('a value of 1 to 3 characters may still be typed', () => {
  assert.deepEqual(findPlain('Router password: hu').map((f) => [f.value, f.partial]), [['hu', true]]);
  assert.deepEqual(findPlain('Router password: ***'), []);
  assert.deepEqual(findPlain('Router password: hunter2').map((f) => f.partial), [false]);
});

test('labels', () => {
  assert.equal(isLabel('Router password'), true);
  assert.equal(isLabel('**Secret**'), true);
  assert.equal(isLabel('1. pwd'), true);
  assert.equal(isLabel('Topping'), false);
  assert.equal(isLabel('The long name of the password of the router'), false);
});
