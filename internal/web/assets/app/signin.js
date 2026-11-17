// SPDX-License-Identifier: MIT
// Passkeys on the pages of signing in (ADR-23): the server sends the options of a ceremony as JSON, the browser asks the
// authenticator, and the answer goes back as JSON. Binary values travel as base64url both ways.
'use strict';
(() => {
  const toBytes = (s) => {
    const b64 = s.replace(/-/g, '+').replace(/_/g, '/') + '==='.slice((s.length + 3) % 4);
    return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
  };
  const toText = (buf) => {
    let s = '';
    for (const b of new Uint8Array(buf)) s += String.fromCharCode(b);
    return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  };
  const show = (text) => {
    const box = document.querySelector('[data-passkey-error]');
    if (!box) return;
    box.querySelector('section').textContent = text;
    box.classList.toggle('hidden', !text);
  };
  const post = async (url, body) => {
    const r = await fetch(url, {method: 'POST', credentials: 'same-origin', headers: {'Content-Type': 'application/json', Accept: 'application/json'},
      body: body === undefined ? '' : JSON.stringify(body)});
    let data = {};
    try { data = await r.json(); } catch (e) { data = {}; }
    if (!r.ok) throw new Error(data.error || 'The server said no (' + r.status + ').');
    return data;
  };
  const failed = (e) => {
    if (e && e.name === 'NotAllowedError') show('The passkey was not used. Try again.');
    else show((e && e.message) || 'That did not work.');
  };

  const signIn = async (button) => {
    show('');
    button.disabled = true;
    try {
      const options = (await post('/sign-in/passkey/options')).publicKey;
      options.challenge = toBytes(options.challenge);
      (options.allowCredentials || []).forEach((c) => { c.id = toBytes(c.id); });
      const cred = await navigator.credentials.get({publicKey: options});
      const next = button.dataset.next || '/';
      const done = await post('/sign-in/passkey?next=' + encodeURIComponent(next), {
        id: cred.id, rawId: toText(cred.rawId), type: cred.type,
        response: {
          clientDataJSON: toText(cred.response.clientDataJSON), authenticatorData: toText(cred.response.authenticatorData),
          signature: toText(cred.response.signature), userHandle: cred.response.userHandle ? toText(cred.response.userHandle) : null,
        },
      });
      location.assign(done.next || '/');
    } catch (e) {
      failed(e);
    } finally {
      button.disabled = false;
    }
  };

  const create = async (button) => {
    show('');
    button.disabled = true;
    try {
      const options = (await post(button.dataset.options)).publicKey;
      options.challenge = toBytes(options.challenge);
      options.user.id = toBytes(options.user.id);
      (options.excludeCredentials || []).forEach((c) => { c.id = toBytes(c.id); });
      const cred = await navigator.credentials.create({publicKey: options});
      const field = document.getElementById(button.dataset.nameFrom || '');
      const name = field ? field.value.trim() : '';
      const done = await post(button.dataset.finish + '?name=' + encodeURIComponent(name), {
        id: cred.id, rawId: toText(cred.rawId), type: cred.type,
        response: {
          clientDataJSON: toText(cred.response.clientDataJSON), attestationObject: toText(cred.response.attestationObject),
          transports: cred.response.getTransports ? cred.response.getTransports() : [],
        },
      });
      location.assign(done.next || '/');
    } catch (e) {
      failed(e);
    } finally {
      button.disabled = false;
    }
  };

  document.addEventListener('click', (event) => {
    const button = event.target.closest('[data-passkey-sign-in], [data-passkey-create]');
    if (!button) return;
    event.preventDefault();
    if (!window.PublicKeyCredential) {
      show('This browser has no passkeys.');
      return;
    }
    if (button.hasAttribute('data-passkey-sign-in')) signIn(button);
    else create(button);
  });
  // The buttons work from here on; a test waits for this mark before it clicks.
  document.documentElement.dataset.passkeys = 'ready';
})();
