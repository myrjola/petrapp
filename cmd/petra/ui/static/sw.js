// Service worker for Petra. Owns Web Push delivery only — no offline caching.

self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));

self.addEventListener('push', (event) => {
    let payload = {};
    try {
        if (event.data) {
            payload = event.data.json();
        }
    } catch (_) {
        // ignore malformed payloads; fall back to defaults below
    }
    const title = payload.title || 'Rest over';
    const body = payload.body || 'Time for your next set.';
    const tag = payload.exercise_name ? `rest-${payload.exercise_name}` : 'rest';

    event.waitUntil(self.registration.showNotification(title, {
        body,
        tag,
        renotify: true,
        icon: '/apple-touch-icon.png',
        badge: '/logo.svg',
        data: payload,
    }));
});

// pushsubscriptionchange fires when the browser rotates or expires our push
// subscription (routine on iOS). The old endpoint the server stored is now
// dead; re-subscribe and POST the fresh subscription so the server's stored
// endpoint can't silently drift from the device's — otherwise the next push
// 410s, the row is pruned, and rest pings stop forever with no recovery. The
// VAPID key can't be read from a page here (there may be none), so fetch it.
self.addEventListener('pushsubscriptionchange', (event) => {
    event.waitUntil(resubscribeAndSync());
});

async function resubscribeAndSync() {
    const keyResp = await fetch('/api/push/vapid-public-key');
    if (!keyResp.ok) throw new Error('vapid key HTTP ' + keyResp.status);
    const vapidKey = (await keyResp.text()).trim();
    const sub = await self.registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: b64ToUint8(vapidKey),
    });
    const syncResp = await fetch('/api/push/subscribe', {
        method: 'POST',
        headers: {'Content-Type': 'application/json', 'X-Requested-With': 'fetch'},
        body: JSON.stringify(sub),
    });
    if (!syncResp.ok) throw new Error('subscribe HTTP ' + syncResp.status);
}

function b64ToUint8(b64) {
    const padding = '='.repeat((4 - b64.length % 4) % 4);
    const base64 = (b64 + padding).replace(/-/g, '+').replace(/_/g, '/');
    const raw = atob(base64);
    return Uint8Array.from([...raw].map((c) => c.charCodeAt(0)));
}

self.addEventListener('notificationclick', (event) => {
    event.notification.close();
    const targetPath = '/';
    event.waitUntil((async () => {
        const clientList = await self.clients.matchAll({type: 'window', includeUncontrolled: true});
        for (const client of clientList) {
            if ('focus' in client) {
                await client.focus();
                return;
            }
        }
        if (self.clients.openWindow) {
            await self.clients.openWindow(targetPath);
        }
    })());
});
