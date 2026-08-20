import { useCallback, useEffect, useState } from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import { getInfo, probeAuth, hasToken, clearToken } from "./lib/storeApi";
import Login from "./pages/Login";
import Catalog from "./pages/Catalog";
import RepoDetail from "./pages/RepoDetail";
import { ThemeToggle } from "@/components/ThemeToggle";
import { Button } from "@/components/ui/button";

type Gate = "loading" | "login" | "ready";

export default function App() {
  const [gate, setGate] = useState<Gate>("loading");
  const [version, setVersion] = useState("");

  const evaluate = useCallback(async () => {
    setGate("loading");
    try {
      const info = await getInfo();
      setVersion(info.version);
      if (info.anon_pull && !hasToken()) {
        setGate("ready");
        return;
      }
      const ok = await probeAuth();
      setGate(ok ? "ready" : "login");
    } catch {
      setGate("login");
    }
  }, []);

  useEffect(() => {
    void evaluate();
  }, [evaluate]);

  if (gate === "loading") return <p className="p-6 text-sm text-muted-foreground">Loading…</p>;
  if (gate === "login") return <Login onAuthed={() => void evaluate()} />;

  return (
    <div className="flex h-screen flex-col">
      <header className="flex items-center justify-between border-b px-4 py-2">
        <a href="/" className="font-semibold">tariboy-store</a>
        <div className="flex items-center gap-2">
          {version && <span className="text-xs text-muted-foreground">v{version}</span>}
          {hasToken() && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                clearToken();
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
