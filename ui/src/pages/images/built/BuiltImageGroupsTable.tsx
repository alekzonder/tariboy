import { Link } from "react-router-dom";
import { builtDate, type BuiltImageGroup } from "./builtImages";

const columns = [
  "Image",
  "Latest image_version",
  "Latest built at",
  "Newest built tag",
  "Tag built at",
];

export function BuiltImageGroupsTable({
  groups,
  basePath,
}: {
  groups: BuiltImageGroup[];
  basePath: string;
}) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            {columns.map((label) => (
              <th key={label} className="px-3 py-2">
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {groups.map(({ name, tags, latest }) => (
            <tr key={name} className="border-t">
              <td className="px-3 py-2">
                <Link
                  className="font-mono text-primary hover:underline"
                  to={`${basePath}/${encodeURIComponent(name)}`}
                >
                  {name}
                </Link>
              </td>
              <td className="px-3 py-2">
                {latest ? latest.image_version || "Version not specified" : "No latest"}
              </td>
              <td className="px-3 py-2 text-xs">{builtDate(latest)}</td>
              <td className="px-3 py-2">
                <Link
                  className="font-mono text-primary hover:underline"
                  to={`${basePath}/${encodeURIComponent(name)}/${encodeURIComponent(tags[0].tag)}`}
                >
                  {tags[0].tag}
                </Link>
              </td>
              <td className="px-3 py-2 text-xs">{builtDate(tags[0])}</td>
            </tr>
          ))}
          {groups.length === 0 && (
            <tr>
              <td colSpan={5} className="px-3 py-8 text-center text-muted-foreground">
                No built images on this host.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
