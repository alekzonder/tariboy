import { Link, useParams } from "react-router-dom";
import BuiltImages from "./BuiltImages";

export default function ImageTags({ hostId, basePath }: { hostId: string; basePath: string }) {
  const { name = "" } = useParams();
  return <div className="h-full min-h-0 space-y-4 overflow-y-auto p-6">
    <nav aria-label="Image breadcrumbs" className="flex gap-2 text-sm">
      <Link className="text-primary hover:underline" to={basePath}>Images</Link><span>/</span><span>{name}</span>
    </nav>
    <h1 className="text-lg font-semibold">{name}</h1>
    <BuiltImages key={hostId} hostId={hostId} basePath={basePath} imageName={name} />
  </div>;
}
