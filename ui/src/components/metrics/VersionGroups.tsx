import { Link } from "react-router-dom";
import type { MetricsSlice } from "@/lib/api";
import { VERSION_HINT, byVersionDesc, versionLabel, workflowLabel } from "@/lib/metricsFormat";
import { defaultPair, olderVersion } from "@/lib/metricsQuery";
import { cn } from "@/lib/utils";
import { MIXED_TASKS, SLICE_COLUMNS, TASKS_DONE } from "./sliceColumns";
import { SliceTable, SortBySelect } from "./SliceTable";
import { useSort, type Column } from "./sliceSort";
import { Unknown } from "./Unknown";

type Kind = "image" | "workflow";

const COLUMNS = [TASKS_DONE, MIXED_TASKS, ...SLICE_COLUMNS];
/** Names with no versions to compare. */
const SPECIAL_WORKFLOWS = new Set(["none", "unknown"]);

function groupHint(kind: Kind, name: string): string | undefined {
  if (kind !== "workflow") return undefined;
  if (name === "none") return VERSION_HINT.noWorkflow;
  if (name === "unknown") return VERSION_HINT.unknownWorkflow;
  return undefined;
}

function versionColumn(
  versions: string[],
  rowCompare: ((version: string) => string) | null,
): Column<MetricsSlice> {
  return {
    id: "version",
    label: "Version",
    // unknown sorts last, like every value that is not known.
    value: (s) => (s.version && s.version !== "unknown" ? s.version : null),
    cell: (s) => {
      const version = s.version ?? "";
      if (!version) return <Unknown reason="The version is not known." />;
      const unknown = version === "unknown";
      return (
        <span className="inline-flex items-center gap-2">
          <span
            title={unknown ? VERSION_HINT.unknown : undefined}
            className={cn("font-mono", unknown && "text-muted-foreground")}
          >
            {versionLabel(version)}
          </span>
          {rowCompare && !unknown && versions.length > 1 && (
            <Link
              to={rowCompare(version)}
              aria-label="Compare with another version"
              title="Compare with another version"
              className="rounded px-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              <span aria-hidden="true">⇄</span>
            </Link>
          )}
        </span>
      );
    },
  };
}

/** The Images or Workflows view: one table per name, a row per version, newest
 *  first. A sort applies inside every group. */
export function VersionGroups({
  kind,
  slices,
  compareHref,
}: {
  kind: Kind;
  slices: MetricsSlice[];
  /** The compare screen for versions `a` and `b` of `name`. */
  compareHref: (name: string, a: string, b: string) => string;
}) {
  const { sort, setSort, toggle } = useSort({ id: "version", dir: "desc" });
  const groups = new Map<string, MetricsSlice[]>();
  for (const slice of slices) {
    const name = slice.name ?? "";
    groups.set(name, [...(groups.get(name) ?? []), slice]);
  }
  const names = [...groups.keys()].sort((a, b) => a.localeCompare(b));
  const plural = kind === "image" ? "images" : "workflows";
  return (
    <div className="space-y-4">
      {kind === "workflow" && (
        <p className="text-xs text-muted-foreground">
          A task counts for a version only when all its visits used that version.
        </p>
      )}
      <SortBySelect
        columns={[versionColumn([], null), ...COLUMNS]}
        sort={sort}
        onChange={setSort}
      />
      {names.length === 0 && (
        <p className="rounded border px-3 py-4 text-center text-sm text-muted-foreground">
          No {plural} in this period.
        </p>
      )}
      {names.map((name) => {
        const rows = groups.get(name) ?? [];
        const versions = rows.map((r) => r.version ?? "").sort(byVersionDesc);
        const comparable = !(kind === "workflow" && SPECIAL_WORKFLOWS.has(name));
        const label = kind === "workflow" ? workflowLabel(name) : name;
        const pair = defaultPair(versions);
        return (
          <section key={name} className="space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <h3 title={groupHint(kind, name)} className="text-sm font-medium">
                {label}
              </h3>
              {comparable && versions.length > 1 && (
                <Link
                  to={compareHref(name, pair.a, pair.b)}
                  className="text-xs text-primary hover:underline"
                >
                  Compare versions
                </Link>
              )}
            </div>
            <SliceTable
              label={label}
              rows={rows}
              rowKey={(r) => r.version ?? ""}
              head={
                kind === "workflow" && name === "none"
                  ? undefined
                  : versionColumn(
                      versions,
                      comparable ? (v) => compareHref(name, olderVersion(versions, v), v) : null,
                    )
              }
              columns={COLUMNS}
              sort={sort}
              onSort={toggle}
              empty={`No ${plural} in this period.`}
            />
          </section>
        );
      })}
    </div>
  );
}
