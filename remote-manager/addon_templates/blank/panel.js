/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

(function () {
  'use strict';
  const $ = (id) => document.getElementById(id);

  wrm.on('context', async (ctx) => {
    $('hello').textContent = wrm.t('Hello, ', 'Pozdrav, ') + (ctx.user.name || '') + '!';
    $('note-label').textContent = wrm.t('Note (saved in WRM)', 'Bilješka (sprema se u WRM)');
    $('save').textContent = wrm.t('Save', 'Spremi');
    $('note').value = await wrm.storeGet('note', '');
  });

  $('save').addEventListener('click', async () => {
    try {
      await wrm.storeSet('note', $('note').value);
      wrm.notify(wrm.t('Saved', 'Spremljeno'), 'success');
    } catch (e) {
      wrm.error(e.code + ': ' + e.message);
    }
  });

  wrm.ready();
})();
