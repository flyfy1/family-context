/* global window, location, document */
// Page routes only: user-entered text, account identifiers and URL queries stay local.
(() => {
  if (location.hostname !== 'family.integ.life' || window.top !== window.self || window.integAnalyticsStarted) return;
  window.integAnalyticsStarted = true;
  const routes = new Set(['/', '/feed', '/space', '/elder', '/settings']);
  const safePath = () => {
    const path = location.hash.slice(1).split(/[?#]/, 1)[0] || '/';
    return routes.has(path) ? path : '/not-found';
  };
  window.dataLayer = window.dataLayer || [];
  function gtag() { window.dataLayer.push(arguments); }
  const page = () => ({page_location: `https://family.integ.life${safePath()}`, page_title: 'Family Daily', page_referrer: ''});
  gtag('js', new Date());
  gtag('config', 'G-1BT6RCD1CZ', {...page(), send_page_view: false, allow_google_signals: false, allow_ad_personalization_signals: false});
  let previous;
  const track = () => {
    if (previous === safePath()) return;
    previous = safePath();
    gtag('set', page());
    gtag('event', 'page_view', {...page(), send_to: 'G-1BT6RCD1CZ'});
  };
  for (const method of ['pushState', 'replaceState']) {
    const original = window.history[method];
    window.history[method] = function(...args) { original.apply(this, args); track(); };
  }
  window.addEventListener('popstate', track);
  window.addEventListener('hashchange', track);
  track();
  const script = document.createElement('script');
  script.async = true;
  script.src = 'https://www.googletagmanager.com/gtag/js?id=G-1BT6RCD1CZ';
  document.head.append(script);
})();
