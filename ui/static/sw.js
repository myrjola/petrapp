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
