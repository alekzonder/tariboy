import { useEffect, useState, type ReactNode } from "react";
import { SlidersHorizontal, X } from "lucide-react";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { byVersionDesc, versionLabel, workflowLabel } from "@/lib/metricsFormat";
import {
  FILTER_NAMES,
  METRICS_PERIODS,
  splitRef,
  type MetricsFilters,
  type MetricsPeriod,
  type MetricsWindow,
} from "@/lib/metricsQuery";
import { cn } from "@/lib/utils";
import { ActivityPeriodBar } from "@/pages/activity/ActivityParts";

const SELECT = "h-7 max-w-48 min-w-0 rounded-control border bg-background px-2 text-body";

type Option = { value: string; label: string };

function FilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: Option[];
  onChange: (value: string) => void;
}) {
  // A value from the URL that the lists do not know (a removed agent) stays selectable.
  const all = options.some((o) => o.value === value)
    ? options
    : [...options, { value, label: value }];
  return (
    <select
      aria-label={label}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={SELECT}
    >
      {all.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

/** The image and workflow names and versions of the host's Library. */
function useLibraryOptions() {
  const api = useApi();
  const [images, setImages] = useState<Map<string, string[]>>(new Map());
  const [workflows, setWorkflows] = useState<Map<string, string[]>>(new Map());
  useEffect(() => {
    let live = true;
    const group = (rows: Array<{ name: string; version?: string }>) => {
      const map = new Map<string, string[]>();
      for (const { name, version } of rows) {
        const versions = map.get(name) ?? [];
        if (version && !versions.includes(version)) versions.push(version);
        map.set(name, versions.sort(byVersionDesc));
      }
      return map;
    };
    // A failed list leaves its filter with what the URL holds.
    api.images.listImages().then(
      (r) =>
        live && setImages(group(r.images.map((i) => ({ name: i.name, version: i.image_version })))),
      () => {},
    );
    api.workflows.listWorkflowImages().then(
      (rows) => live && setWorkflows(group(rows)),
      () => {},
    );
    return () => {
      live = false;
    };
  }, [api]);
  return { images, workflows };
}

const names = (map: Map<string, string[]>) => [...map.keys()].sort((a, b) => a.localeCompare(b));

function FilterSelects({
  agents,
  filters,
  onFilters,
}: {
  agents: string[];
  filters: MetricsFilters;
  onFilters: (changes: Partial<MetricsFilters>) => void;
}) {
  const { images, workflows } = useLibraryOptions();
  const image = splitRef(filters.image);
  const workflow = splitRef(filters.workflow);
  const ref = (name: string, version: string) => (version ? `${name}:${version}` : name);
  return (
    <>
      <FilterSelect
        label="Agent"
        value={filters.agent}
        options={[
          { value: "", label: "All agents" },
          ...agents.map((a) => ({ value: a, label: a })),
        ]}
        onChange={(agent) => onFilters({ agent })}
      />
      <FilterSelect
        label="Image"
        value={image.name}
        options={[
          { value: "", label: "All images" },
          ...names(images).map((n) => ({ value: n, label: n })),
        ]}
        onChange={(name) => onFilters({ image: name })}
      />
      {image.name && (
        <FilterSelect
          label="Image version"
          value={image.version}
          options={[
            { value: "", label: "All versions" },
            ...(images.get(image.name) ?? []).map((v) => ({ value: v, label: v })),
            { value: "unknown", label: versionLabel("unknown") },
          ]}
          onChange={(version) => onFilters({ image: ref(image.name, version) })}
        />
      )}
      <FilterSelect
        label="Workflow"
        value={workflow.name}
        options={[
          { value: "", label: "All workflows" },
          { value: "none", label: workflowLabel("none") },
          ...names(workflows).map((n) => ({ value: n, label: n })),
        ]}
        onChange={(name) => onFilters({ workflow: name })}
      />
      {workflow.name && workflow.name !== "none" && (
        <FilterSelect
          label="Workflow version"
          value={workflow.version}
          options={[
            { value: "", label: "All versions" },
            ...(workflows.get(workflow.name) ?? []).map((v) => ({ value: v, label: v })),
          ]}
          onChange={(version) => onFilters({ workflow: ref(workflow.name, version) })}
        />
      )}
    </>
  );
}

const CLEARED: MetricsFilters = { agent: "", image: "", workflow: "" };

/** The row above the numbers: host, period and filters. Below 1280 px the
 *  filters sit behind Filters (N), so the row never wraps on a desktop.
 *  Below 640 px the period takes a first row of its own; up to 1024 px the
 *  row wraps when it does not fit. */
export function MetricsToolbar({
  hostControl,
  window,
  onPeriod,
  onCustom,
  agents,
  filters,
  onFilters,
}: {
  hostControl?: ReactNode;
  window: MetricsWindow;
  onPeriod: (period: MetricsPeriod) => void;
  onCustom: (bound: "since" | "until", value: string) => void;
  agents: string[];
  filters: MetricsFilters;
  onFilters: (changes: Partial<MetricsFilters>) => void;
}) {
  const [open, setOpen] = useState(false);
  const active = FILTER_NAMES.filter((name) => filters[name]).length;
  return (
    <div
      role="toolbar"
      aria-label="Metrics filters"
      data-toolbar
      className="relative flex flex-wrap items-center gap-2 lg:flex-nowrap"
    >
      {hostControl && (
        <div className="flex min-w-0 shrink items-center *:min-w-0 *:shrink">{hostControl}</div>
      )}
      <div className="max-sm:order-first max-sm:basis-full sm:scrollbar-none sm:min-w-0 sm:shrink sm:overflow-x-auto">
        <ActivityPeriodBar
          window={window}
          onPeriod={onPeriod}
          onCustom={onCustom}
          periods={METRICS_PERIODS}
        />
      </div>
      <Button
        type="button"
        size="sm"
        variant="outline"
        aria-expanded={open}
        aria-controls="metrics-filter-list"
        onClick={() => setOpen((v) => !v)}
        className="shrink-0 xl:hidden"
      >
        <SlidersHorizontal aria-hidden="true" />
        {active ? `Filters (${active})` : "Filters"}
      </Button>
      <div
        id="metrics-filter-list"
        onKeyDown={(e) => e.key === "Escape" && setOpen(false)}
        className={cn(
          open ? "flex" : "hidden",
          "items-center gap-2 xl:flex xl:shrink-0",
          "max-xl:absolute max-xl:top-full max-xl:right-0 max-xl:z-sticky max-xl:mt-1 max-xl:flex-col max-xl:items-stretch max-xl:rounded-chip max-xl:border max-xl:bg-popover max-xl:p-2 max-xl:shadow-lift",
        )}
      >
        <FilterSelects agents={agents} filters={filters} onFilters={onFilters} />
      </div>
      {active > 0 && (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => onFilters(CLEARED)}
          className="shrink-0"
        >
          Clear filters
        </Button>
      )}
    </div>
  );
}

function chipLabel(name: (typeof FILTER_NAMES)[number], value: string): string {
  if (name === "agent") return `Agent: ${value}`;
  if (name === "image") return `Image: ${value}`;
  return `Workflow: ${value === "none" ? workflowLabel("none") : value}`;
}

/** The active filters, each removable, and the daemon's word on a bad one. */
export function FilterChips({
  filters,
  onFilters,
  invalid,
}: {
  filters: MetricsFilters;
  onFilters: (changes: Partial<MetricsFilters>) => void;
  invalid: boolean;
}) {
  const active = FILTER_NAMES.filter((name) => filters[name]);
  if (!active.length) return null;
  return (
    <div className="flex flex-wrap items-center gap-2">
      {active.map((name) => {
        const label = chipLabel(name, filters[name]);
        return (
          <button
            key={name}
            type="button"
            aria-label={`Remove filter ${label}`}
            onClick={() => onFilters({ [name]: "" })}
            className={cn(
              "inline-flex max-w-full items-center gap-1 rounded-full border px-2 py-0.5 text-ui hover:bg-accent",
              invalid && "border-destructive",
            )}
          >
            <span className="truncate">{label}</span>
            <X aria-hidden="true" className="size-3 shrink-0" />
          </button>
        );
      })}
      {invalid && (
        <span role="alert" className="text-ui text-destructive">
          This filter is not valid. Use NAME or NAME:VERSION.
        </span>
      )}
    </div>
  );
}
