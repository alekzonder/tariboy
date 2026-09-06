import { expect, test, waitForMainWindow } from "./fixture";

test("Judge table shows start time and linked IDs, newest first", async ({ desktop }) => {
  await waitForMainWindow(desktop);
  // Only the list response is controlled; the production SPA and native WebView
  // render the table. All other requests use the fixture's isolated daemon.
  await desktop.execute(`
    const fetchOriginal = window.fetch.bind(window);
    window.fetch = (input, init) => {
      if (new URL(String(input), location.href).pathname === '/api/judges') {
        const runs = [
          { id: 'older-run', created_at: '2026-09-06T12:00:00Z' },
          { id: 'newer-run', created_at: '2026-09-06T14:30:00+02:00' }
        ].map(run => ({ ...run, original_request: 'Long criteria should stay in details',
          status: 'completed', targets_ready: 0, targets_total: 0,
          assignments_completed: 0, assignments_total: 0 }));
        return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { runs, count: 2 } })));
      }
      return fetchOriginal(input, init);
    };
    window.location.hash = '#/servers/local/settings/advanced/judges';
  `);
  await expect.poll(() => desktop.execute<string[]>(`
    return [...document.querySelectorAll('tbody tr a')].map(a => a.textContent);
  `)).toEqual(["newer-run", "older-run"]);
  expect(await desktop.execute<string[]>(`
    return [...document.querySelectorAll('thead th')].map(th => th.textContent);
  `)).toEqual(["Started", "ID", "Count", "Coverage", "Verdict", "Model", "Cost", "Creator"]);
  expect(await desktop.execute<boolean>(`
    const time = document.querySelector('tbody tr time');
    return time?.getAttribute('datetime') === '2026-09-06T14:30:00+02:00'
      && time.textContent === new Date('2026-09-06T12:30:00Z').toLocaleString()
      && !document.body.innerText.includes('Long criteria should stay in details');
  `)).toBe(true);
  expect(await desktop.execute<string>(`
    return document.querySelector('tbody tr a').getAttribute('href');
  `)).toBe("#/settings/advanced/judges/newer-run");
});
