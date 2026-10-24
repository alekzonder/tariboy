import type { ImageValidationResult } from "@/lib/api";

type ImageTemplate = NonNullable<ImageValidationResult["template"]>;

function ValidatedSkills({ skills }: { skills: NonNullable<ImageValidationResult["skills"]> }) {
  return (
    <div className="space-y-1" aria-label="Validated image skills">
      <div className="text-sm">Skills: {skills.length === 0 ? "none" : null}</div>
      {skills.map((skill) => (
        <div key={skill.name} className="rounded border bg-background p-2 text-sm">
          <strong>{skill.name}</strong>
          <p>{skill.description}</p>
          <div className="break-all font-mono text-xs">
            {skill.category} · {skill.source}
          </div>
          <div className="break-all text-xs text-muted-foreground">
            {skill.file_count} files · {skill.size} bytes · {skill.tree_sha256}
          </div>
        </div>
      ))}
    </div>
  );
}

function TemplateEntries({ entries }: { entries: ImageTemplate["entries"] }) {
  return (
    <ol className="space-y-1">
      {entries.map((entry, index) => (
        <li
          key={`${index}-${entry.archive_path ?? entry.runtime}`}
          className="rounded border bg-background p-2 text-sm"
        >
          <span className="mr-3 font-mono text-muted-foreground">{index}</span>
          <strong className="mr-2">{entry.kind}</strong>
          {entry.runtime && <code>{entry.runtime}</code>}
          {entry.source && <div className="break-all font-mono text-xs">{entry.source}</div>}
          {entry.category && (
            <div className="text-xs text-muted-foreground">
              {entry.category} · {entry.size ?? 0} bytes · {entry.sha256}
            </div>
          )}
        </li>
      ))}
    </ol>
  );
}

export function ValidatedTemplate({ validation }: { validation: ImageValidationResult }) {
  return (
    <div className="space-y-2 rounded border bg-muted/20 p-3" aria-label="Validated image template">
      <div className="text-xs text-muted-foreground">
        schema v{validation.schema_version} · template sha256{" "}
        <span className="font-mono">{validation.template?.sha256 ?? "legacy"}</span>
      </div>
      <div className="text-sm">
        Plugins:{" "}
        {(validation.plugins ?? []).length === 0
          ? "none"
          : validation.plugins!.map((plugin) => (
              <code key={plugin} className="ml-2">
                {plugin}
              </code>
            ))}
      </div>
      <ValidatedSkills skills={validation.skills ?? []} />
      {validation.template && <TemplateEntries entries={validation.template.entries} />}
    </div>
  );
}
