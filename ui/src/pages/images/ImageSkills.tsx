import { useImageContext } from "@/components/ImageLayout";

export default function ImageSkills() {
  const { manifest } = useImageContext();
  if (!manifest) return <p className="text-sm text-muted-foreground">Loading…</p>;
  const skills = manifest.skills ?? [];
  if (skills.length === 0) {
    return <p className="text-sm text-muted-foreground">This image contains no packaged skills.</p>;
  }
  return <ol className="space-y-3">
    {skills.map((skill) => <li key={skill.name} className="space-y-2 rounded border p-3 text-sm">
      <div>
        <strong>{skill.name}</strong>
        <p>{skill.description}</p>
      </div>
      <div className="break-all font-mono text-xs">{skill.source}</div>
      <div className="text-xs text-muted-foreground">{skill.category} · {skill.archive_root}</div>
      <div className="text-xs text-muted-foreground">{skill.file_count} files · {skill.size} bytes</div>
      <div className="break-all font-mono text-xs">{skill.tree_sha256}</div>
    </li>)}
  </ol>;
}
