import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";

// Temporary guard of stage 4 (spec 9.2): every design-sync preview has a story
// file with the same export names, so no state is lost. The last commit of the
// stage deletes this file with the previews. CustomerQuestionNotifications is
// not migrated (spec 9.1).
const ui = resolve(__dirname, "../..");
const previews = resolve(ui, ".design-sync/previews");

const C = "src/components";
const U = "src/components/ui";
const MAP: Record<string, string> = {
  AgentControls: `${C}/AgentControls.stories.tsx`,
  AgentRow: `${C}/AgentRow.stories.tsx`,
  AgentWorkspace: "src/pages/agents/AgentWorkspace.stories.tsx",
  AlertDialog: `${U}/alert-dialog.stories.tsx`,
  AppSettings: `${C}/DesktopUpdates.stories.tsx`,
  AuditExportActions: `${C}/AuditExportActions.stories.tsx`,
  AuditRow: `${C}/AuditRow.stories.tsx`,
  Badge: `${U}/badge.stories.tsx`,
  Button: `${U}/button.stories.tsx`,
  Card: `${U}/card.stories.tsx`,
  Collapsible: `${U}/collapsible.stories.tsx`,
  ComboField: `${C}/ComboField.stories.tsx`,
  Command: `${U}/command.stories.tsx`,
  ContextMenu: `${U}/context-menu.stories.tsx`,
  DaemonBanner: `${C}/DaemonBanner.stories.tsx`,
  Dialog: `${U}/dialog.stories.tsx`,
  DropdownMenu: `${U}/dropdown-menu.stories.tsx`,
  EditablePresetCombobox: `${C}/EditablePresetCombobox.stories.tsx`,
  FileBrowser: `${C}/FileBrowser.stories.tsx`,
  FullAuditLog: `${C}/FullAuditLog.stories.tsx`,
  GoalHelp: `${C}/GoalHelp.stories.tsx`,
  GroupWizard: `${C}/GroupWizard.stories.tsx`,
  HostStatus: `${C}/HostStatus.stories.tsx`,
  ImageCombobox: `${C}/AgentFormFields.stories.tsx`,
  ImageLayout: `${C}/ImageLayout.stories.tsx`,
  Input: `${U}/input.stories.tsx`,
  IterationAuditLog: `${C}/IterationAuditLog.stories.tsx`,
  Label: `${U}/label.stories.tsx`,
  LibraryImagesPage: "src/pages/images/LibraryImagesPage.stories.tsx",
  PathAutocomplete: `${C}/PathAutocomplete.stories.tsx`,
  PriorityTag: `${U}/status.stories.tsx`,
  ProxyCallRow: `${C}/ProxyCallRow.stories.tsx`,
  ScrollArea: `${U}/scroll-area.stories.tsx`,
  Segmented: `${U}/segmented.stories.tsx`,
  Select: `${U}/select.stories.tsx`,
  SendFilesButton: `${C}/SendFilesButton.stories.tsx`,
  Separator: `${U}/separator.stories.tsx`,
  SettingsPage: "src/pages/settings/SettingsPage.stories.tsx",
  Sheet: `${U}/sheet.stories.tsx`,
  ShellScriptEditor: `${C}/ShellScriptEditor.stories.tsx`,
  StatusDot: `${U}/status.stories.tsx`,
  StatusPill: `${U}/status.stories.tsx`,
  StoresPage: "src/pages/StoresPage.stories.tsx",
  SupportBundle: `${C}/SupportBundle.stories.tsx`,
  Switch: `${U}/switch.stories.tsx`,
  Tabs: `${U}/tabs.stories.tsx`,
  TaskDetail: "src/pages/tasks/TaskDetail.stories.tsx",
  TasksWorkspace: "src/pages/tasks/TasksWorkspace.stories.tsx",
  TerminalsPage: "src/pages/terminals/TerminalsPage.stories.tsx",
  Textarea: `${U}/textarea.stories.tsx`,
  ThemeToggle: `${C}/ThemeToggle.stories.tsx`,
  Toaster: `${U}/sonner.stories.tsx`,
  Tooltip: `${U}/tooltip.stories.tsx`,
  TuiScreen: `${C}/TuiScreen.stories.tsx`,
};

const exportsOf = (path: string): string[] =>
  [...readFileSync(path, "utf8").matchAll(/^export const (\w+)/gm)].map((m) => m[1]);

it("has a story with every export of every design-sync preview", () => {
  expect(Object.keys(MAP)).toHaveLength(54);
  const missingFiles = [...new Set(Object.values(MAP))].filter(
    (file) => !existsSync(resolve(ui, file)),
  );
  const missingExports: string[] = [];
  for (const [name, file] of Object.entries(MAP)) {
    if (missingFiles.includes(file)) continue;
    const have = new Set(exportsOf(resolve(ui, file)));
    for (const exported of exportsOf(resolve(previews, `${name}.tsx`))) {
      if (!have.has(exported)) missingExports.push(`${file}: ${name} export ${exported}`);
    }
  }
  expect({ missingFiles, missingExports }).toEqual({ missingFiles: [], missingExports: [] });
});
