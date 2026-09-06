import { spawnSync } from "node:child_process";
import { access, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { expect, test, waitForMainWindow } from "./fixture";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");

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
  `)).toBe("#/servers/local/settings/advanced/judges/newer-run");
});

test("reviews real iterations and keeps navigation on the fixture daemon", async ({ desktop, desktopWorker }) => {
  const agents = ["judge-one", "judge-two"];
  const cli = join(repositoryRoot, "desktop/src-tauri/resources/bin/linux-x86_64/tariboy");
  const version = spawnSync(cli, ["version"], { encoding: "utf8" });
  expect(version.status, version.error?.message ?? version.stderr).toBe(0);
  const clientVersion = version.stdout.trim();
  agents.forEach(desktopWorker.registerAgentForCleanup);
  await waitForMainWindow(desktop);

  await desktop.execute(`
    window.__judgeSetup = "running";
    window.__TAURI_INTERNALS__.invoke("daemon_status").then(async (status) => {
      window.__judgeBaseURL = status.base_url;
      window.__judgeRequests = [];
      const originalFetch = window.fetch.bind(window);
      window.fetch = (input, init) => {
        const url = new URL(typeof input === "string" ? input : input.url, location.href);
        if (url.pathname.startsWith("/api/")) window.__judgeRequests.push(url.href);
        return originalFetch(input, init);
      };
      const call = async (method, path, body) => {
        const response = await fetch(status.base_url + path, {
          method,
          headers: body === undefined ? undefined : { "content-type": "application/json" },
          body: body === undefined ? undefined : JSON.stringify(body),
        });
        const envelope = await response.json();
        if (!response.ok || !envelope.ok) throw new Error(path + ": " + JSON.stringify(envelope.error));
        return envelope.result;
      };
      window.__judgeCall = call;
      await call("POST", "/api/images/build", {
        name: "judge-e2e", tag: "latest",
        path: ${JSON.stringify(join(repositoryRoot, "store/images/llm-as-judge"))},
      });
      await call("POST", "/api/agents", { name: "judge-target", image: "basic:latest", harness: "stub", loop: false });
      for (const name of ["judge-lead", "judge-one", "judge-two"]) {
        await call("POST", "/api/agents", {
          name, image: "judge-e2e:latest", harness: "stub", interactive: false, loop: false,
          env: "STUB_SLEEP=300,STUB_CALL_DONE=0",
        });
      }
      await call("POST", "/api/groups", { name: "judge-e2e", lead: "judge-lead" });
      for (const name of ["judge-lead", "judge-one", "judge-two"])
        await call("POST", "/api/groups/judge-e2e/assign", { agent: name });
      const iterations = [];
      for (const prompt of ["first fixture iteration", "second fixture iteration"]) {
        await call("POST", "/api/agents/judge-target/exec", { prompt });
        const deadline = Date.now() + 30000;
        while (Date.now() < deadline) {
          const rows = (await call("GET", "/api/agents/judge-target/iterations")).iterations;
          const latest = rows[rows.length - 1];
          if (latest && latest.status !== "running" && !iterations.includes(latest.id)) {
            iterations.push(latest.id); break;
          }
          await new Promise(resolve => setTimeout(resolve, 100));
        }
      }
      if (iterations.length !== 2) throw new Error("target iterations did not finish");
      await call("PUT", "/api/judge-automation", { config_json: JSON.stringify({
        schema_version: 1, enabled: false,
        judge: { lead: "judge-lead", workers: ["judge-one", "judge-two"], image_ref: "judge-e2e:latest" },
        schedule: { spec: "0 0 1 1 *" },
        targets: { agents: ["judge-target"], image_refs: ["basic:latest"], only_unprocessed: false },
      }) });
      for (const name of ["judge-lead", "judge-one", "judge-two"]) {
        await call("POST", "/api/agents/" + name + "/loop/disable");
      }
      for (const name of ["judge-one", "judge-two"])
        await call("POST", "/api/agents/" + name + "/exec", { prompt: "controlled Judge fixture worker" });
      const automation = await call("GET", "/api/judge-automation");
      const statuses = await Promise.all(["judge-lead", "judge-one", "judge-two"].map(name => call("GET", "/api/agents/" + name + "/status")));
      if (automation.revision === undefined || statuses.some(value => value.loop_enabled)) throw new Error("Judge workers must remain manual");
      window.__judgeIterations = iterations;
      window.location.hash = "#/agents/local/judge-target/activity?iteration=" + encodeURIComponent(iterations[0]);
      window.__judgeSetup = "ready";
    }).catch(error => { window.__judgeSetup = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>("return window.__judgeSetup || '';"), { timeout: 90_000 }).toBe("ready");
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("Run Judge review");
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Run Judge review']"));
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("Queued");

  const worker = async (name: string, run: string, score: number, summary: string) => {
    const socket = join(desktopWorker.runtimeDir, `${name}.sock`);
    await expect.poll(async () => { try { await access(socket); return true; } catch { return false; } }, { timeout: 30_000 }).toBe(true);
    const judge = join(repositoryRoot, "store/skills/llm-as-judge/scripts/judge.sh");
    const env = { ...process.env, TARIBOY_TOOLS_SOCKET: socket, TARIBOY_CLIENT_VERSION: clientVersion };
    const claimed = spawnSync(judge, ["--json", "work", "claim", "--run", run], { env, encoding: "utf8" });
    expect(claimed.status, claimed.error?.message ?? claimed.stderr).toBe(0);
    const assignment = JSON.parse(claimed.stdout).assignment;
    const assignmentID = assignment.ID;
    const evidence = spawnSync(judge, ["--json", "evidence", "search", "--assignment", assignmentID, "--artifact", "metadata"], { env, encoding: "utf8" });
    expect(evidence.status, evidence.stderr).toBe(0);
    const bundleHash = JSON.parse(evidence.stdout).bundle_hash;
    const file = join(desktopWorker.baseDir, `${assignmentID}.json`);
    await writeFile(file, JSON.stringify({ schema_version: 1, verdict: score === 0 ? "fail" : "pass", score, confidence: 1, summary,
      violations: score === 0 ? [{ criterion: "fixture", severity: "high", description: summary, citations: [{ bundle_hash: bundleHash, artifact: "metadata", locator: "metadata" }] }] : [],
      strengths: score === 0 ? [] : [{ description: summary, citations: [{ bundle_hash: bundleHash, artifact: "metadata", locator: "metadata" }] }],
      recommendations: [], evidence_gaps: [],
    }));
    const submitted = spawnSync(judge, ["--json", "analysis", "submit", "--assignment", assignmentID, "--file", file], { env, encoding: "utf8" });
    expect(submitted.status, submitted.stderr).toBe(0);
    return assignment.TargetID;
  };

  const first = await desktop.execute<{ runID: string }>(`
    const iteration = window.__judgeIterations[0];
    const reviews = await window.__judgeCall("GET", "/api/agents/judge-target/iterations/" + encodeURIComponent(iteration) + "/judges");
    return { runID: reviews.reviews[0].run_id };
  `);
  await worker("judge-one", first.runID, 0, "first target zero");
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("Score 0");
  expect(await desktop.execute<boolean>(`
    const detail = await window.__judgeCall("GET", "/api/judges/${first.runID}");
    return detail.run.judges_per_iteration === 1
      && detail.targets[0].assignments_completed === 1
      && detail.targets[0].assignments_pending === 0;
  `)).toBe(true);

  const second = await desktop.execute<{ runID: string }>(`
    const result = await window.__judgeCall("POST", "/api/judges/review", { iteration: window.__judgeIterations, judges_per_iteration: 1 });
    return { runID: result.id };
  `);
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("Score 0");
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("Waiting: Judge worker judge-one has Autopilot disabled.");
  const firstTarget = await worker("judge-one", second.runID, 0, "second run first target");
  const secondTarget = await worker("judge-two", second.runID, 1, "second target only");
  const targetIterations = await desktop.execute<{ first: string; second: string }>(`
    const detail = await window.__judgeCall("GET", "/api/judges/${second.runID}");
    return {
      first: detail.targets.find(target => target.id === ${JSON.stringify(firstTarget)}).iteration,
      second: detail.targets.find(target => target.id === ${JSON.stringify(secondTarget)}).iteration,
    };
  `);

  await desktop.execute(`window.location.hash = "#/servers/local/settings/advanced/judges/${second.runID}?target=${secondTarget}"; return true;`);
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("second target only");
  expect(await desktop.execute<string>("return document.body.innerText")).not.toContain("second run first target");
  expect(await desktop.execute<string[]>(`return [...document.querySelectorAll('nav[aria-label="Breadcrumb"] a')].map(a => a.textContent);`)).toEqual(["Judge runs", expect.stringContaining("judge-target iteration")]);
  await desktop.elementClick(await desktop.findElement("xpath", "//button[contains(normalize-space(.), '[metadata:metadata]')]"));
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("Immutable evidence (untrusted)");
  const evidenceText = await desktop.execute<string>(`
    return [...document.querySelectorAll("span")]
      .find(element => element.textContent.startsWith("Immutable evidence (untrusted):"))?.textContent || "";
  `);
  expect(evidenceText).toContain(targetIterations.second);
  expect(evidenceText).not.toContain(targetIterations.first);
  expect(await desktop.execute<boolean>(`
    const base = window.__judgeBaseURL;
    return window.__judgeRequests.length > 0 && window.__judgeRequests.every(url => url.startsWith(base + "/api/"))
      && document.querySelector('nav[aria-label="Breadcrumb"] a')?.getAttribute('href') === '#/servers/local/settings/advanced/judges';
  `)).toBe(true);
  expect(firstTarget).not.toBe(secondTarget);

  await desktop.elementClick(await desktop.findElement("xpath", "//nav[@aria-label='Breadcrumb']/a[contains(., 'judge-target iteration')]"));
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("Judge review");
  await desktop.execute("history.back(); return true;");
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("second target only");
  await desktop.elementClick(await desktop.findElement("xpath", "//nav[@aria-label='Breadcrumb']/a[normalize-space(.)='Judge runs']"));
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain(second.runID);
  await desktop.execute("history.back(); return true;");
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).toContain("second target only");

  const judgeFetches = await desktop.execute<number>(`return window.__judgeRequests.filter(url => new URL(url).pathname === "/api/judges/${second.runID}").length;`);
  await desktop.execute(`window.location.hash = "#/servers/unknown-host/settings/advanced/judges/${second.runID}?target=${secondTarget}"; return true;`);
  await expect.poll(() => desktop.execute<string>("return document.body.innerText"), { timeout: 30_000 }).not.toContain("second target only");
  expect(await desktop.execute<number>(`return window.__judgeRequests.filter(url => new URL(url).pathname === "/api/judges/${second.runID}").length;`)).toBe(judgeFetches);
});
