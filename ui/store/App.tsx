import { useCallback, useEffect, useState } from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import { getInfo, probeAuth, hasToken, clearToken } from "./lib/storeApi";
import Login from "./pages/Login";
import Catalog from "./pages/Catalog";
import RepoDetail from "./pages/RepoDetail";
import { ThemeToggle } from "@/components/ThemeToggle";
import { Button } from "@/components/ui/button";

type Gate = "loading" | "login" | "ready";

async function checkGate(): Promise<{ version?: string; gate: Gate }> {
  let version: string | undefined;
  try {
    const info = await getInfo();
    version = info.version;
    if (info.anon_pull && !hasToken()) return { version, gate: "ready" };
    return { version, gate: (await probeAuth()) ? "ready" : "login" };
  } catch {
    return { version, gate: "login" };
  }
}

export default function App() {
  const [gate, setGate] = useState<Gate>("loading");
  const [version, setVersion] = useState("");

  // The gate starts at "loading"; a re-check after login or logout sets it first.
  const evaluate = useCallback(
    () =>
      checkGate().then((next) => {
        if (next.version !== undefined) setVersion(next.version);
        setGate(next.gate);
      }),
    [],
  );

  useEffect(() => {
    void evaluate();
  }, [evaluate]);

  if (gate === "loading") return <p className="p-6 text-sm text-muted-foreground">Loading…</p>;
  if (gate === "login")
    return (
      <Login
        onAuthed={() => {
          setGate("loading");
          void evaluate();
        }}
      />
    );

  return (
    <div className="flex h-screen flex-col">
      <header className="flex items-center justify-between border-b px-4 py-2">
        <a href="/" className="font-semibold">
          tariboy-store
        </a>
        <div className="flex items-center gap-2">
          {version && <span className="text-xs text-muted-foreground">v{version}</span>}
          {hasToken() && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                clearToken();
                setGate("loading");
                void evaluate();
              }}
            >
              Logout
            </Button>
          )}
          <ThemeToggle />
        </div>
      </header>
      <main className="flex-1 overflow-auto">
        <Routes>
          <Route path="/" element={<Catalog />} />
          <Route path="/repo/:name" element={<RepoDetail />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  );
}
