import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useRef, type ReactNode, type RefObject } from "react";
import FileBrowser, { type FileBrowserApi } from "./FileBrowser";

// FileBrowser is the one browser behind both the agent Files tab (editable) and
// the image Files tab (read-only), and it is source-agnostic on purpose: the
// caller injects listDir/readFile plus the optional write/manage callbacks.
// These stories inject an in-memory tariboy checkout, so the real tree, the
// real CodeMirror viewer and the real toolbar all render with no daemon.
const meta = { component: FileBrowser } satisfies Meta<typeof FileBrowser>;
export default meta;

type Story = StoryObj;

type Entry = { name: string; isDir: boolean; size: number; mtime: number };

const dir = (name: string): Entry => ({ name, isDir: true, size: 0, mtime: 1757606561 });
const file = (name: string, size: number): Entry => ({
  name,
  isDir: false,
  size,
  mtime: 1757606561,
});

const MAIN_GO = `package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/tariboy/tariboy/internal/daemon"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7777", "listen address")
	state := flag.String("state", "/home/agent/.local/state/tariboy", "state directory")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	d, err := daemon.Open(*state, log)
	if err != nil {
		log.Error("open state", "err", err)
		os.Exit(1)
	}
	defer d.Close()

	if err := d.Serve(context.Background(), *addr); err != nil {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
`;

const MANIFEST_JSON = `{
  "ref": "worker:v2",
  "digest": "sha256:9f21c0b4e7d3",
  "base": "bare:latest",
  "entrypoint": ["/usr/local/bin/tariboy-shim"],
  "plugins": [
    { "name": "go", "version": "1.23.2" },
    { "name": "node", "version": "22.8.0" }
  ],
  "env": {
    "TB_CACHE": "/home/agent/.cache/tariboy"
  }
}
`;

const AGENT_TREE: Record<string, Entry[]> = {
  "": [
    dir("cmd"),
    dir("internal"),
    dir("ui"),
    file("AGENTS.md", 4210),
    file("Makefile", 812),
    file("go.mod", 1344),
    file("main.go", 1096),
  ],
  internal: [dir("audit"), dir("daemon"), file("loop.go", 9820), file("retention.go", 3140)],
  cmd: [dir("tariboy"), dir("tariboy-shim")],
  ui: [dir("src"), file("package.json", 2638), file("vite.config.ts", 984)],
};

const IMAGE_TREE: Record<string, Entry[]> = {
  "": [dir("bin"), dir("etc"), file("manifest.json", 486), file("PROMPT.md", 2874)],
  bin: [file("tariboy-shim", 18422400)],
  etc: [file("shell.sh", 640)],
};

const CONTENT: Record<string, string> = {
  "main.go": MAIN_GO,
  "manifest.json": MANIFEST_JSON,
};

const listing = (tree: Record<string, Entry[]>) => (path: string) =>
  Promise.resolve({ path, entries: tree[path] ?? [] });

const read: FileBrowserApi["readFile"] = (path) =>
  Promise.resolve({
    path,
    kind: "text" as const,
    content: CONTENT[path] ?? "",
    size: (CONTENT[path] ?? "").length,
  });

const listAgent = listing(AGENT_TREE);
const listImage = listing(IMAGE_TREE);
const listEmpty = (path: string) => Promise.resolve({ path, entries: [] });
const write = () => Promise.resolve();
const create = () => Promise.resolve();
const rename = () => Promise.resolve();
const remove = () => Promise.resolve();

// Walk the browser the way an operator does: click the real tree rows once the
// injected listing has resolved.
function useTreeClicks(steps: string[]) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let i = 0;
    let tries = 0;
    const id = window.setInterval(() => {
      const root = ref.current;
      if (!root || i >= steps.length || ++tries > 60) {
        window.clearInterval(id);
        return;
      }
      const want = steps[i];
      const btn = Array.from(root.querySelectorAll("button")).find(
        (b) => (b.textContent ?? "").trim() === want,
      );
      if (!btn) return;
      btn.click();
      i += 1;
      tries = 0;
    }, 25);
    return () => window.clearInterval(id);
  }, [steps]);
  return ref;
}

// Module constants keep each step list's identity stable, so the walk runs once.
const workdirSteps = ["internal", "main.go"];
const imageSteps = ["manifest.json"];
const noSteps: string[] = [];

const Pane = ({
  inner,
  height = 480,
  children,
}: {
  inner: RefObject<HTMLDivElement | null>;
  height?: number;
  children: ReactNode;
}) => (
  <div ref={inner} style={{ height }}>
    {children}
  </div>
);

function AgentWorkdirDemo() {
  const ref = useTreeClicks(workdirSteps);
  return (
    <Pane inner={ref}>
      <FileBrowser
        sourceKey="builder"
        listDir={listAgent}
        readFile={read}
        writeFile={write}
        createFile={create}
        renameFile={rename}
        deleteFile={remove}
      />
    </Pane>
  );
}

function ImageFilesDemo() {
  const ref = useTreeClicks(imageSteps);
  return (
    <Pane inner={ref}>
      <FileBrowser sourceKey="worker:v2" listDir={listImage} readFile={read} readOnly />
    </Pane>
  );
}

function EmptyWorkdirDemo() {
  const ref = useTreeClicks(noSteps);
  return (
    <Pane inner={ref} height={220}>
      <FileBrowser
        sourceKey="docs-bot"
        listDir={listEmpty}
        readFile={read}
        writeFile={write}
        createFile={create}
      />
    </Pane>
  );
}

// The agent Files tab: tree on the left, the selected file in CodeMirror with
// the Edit affordance, New file / New folder in the tree header.
export const AgentWorkdir: Story = { render: () => <AgentWorkdirDemo /> };

// The image detail Files tab passes `readOnly`: no create/rename/delete in the
// header or the rows, and the viewer offers no Edit.
export const ImageFilesReadOnly: Story = { render: () => <ImageFilesDemo /> };

// A managed workdir the agent has not written to yet.
export const EmptyWorkdir: Story = { render: () => <EmptyWorkdirDemo /> };
