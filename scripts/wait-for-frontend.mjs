import { setTimeout as delay } from 'node:timers/promises';

const url = new URL(process.env.FRONTEND_DEVSERVER_URL || 'http://localhost:9245');
if (!['localhost', '127.0.0.1', '[::1]'].includes(url.hostname) || !['http:', 'https:'].includes(url.protocol)) {
  throw new Error('The desktop development frontend must use a loopback HTTP endpoint.');
}
// Wails' asset proxy dials tcp4 even when its development URL uses localhost.
if (url.hostname === 'localhost') url.hostname = '127.0.0.1';
const deadline = Date.now() + 45_000;
let ready = false;
while (Date.now() < deadline) {
  try {
    const response = await fetch(url, { signal: AbortSignal.timeout(2000), redirect: 'error' });
    await response.body?.cancel();
    if (response.ok) { ready = true; break; }
  } catch {
    // A cold Vite start may still be optimizing dependencies.
  }
  await delay(250);
}
if (!ready) throw new Error('Desktop frontend did not become ready within 45 seconds. Check the Vite output.');
console.log('Desktop frontend is ready.');
