import { Link, NavLink, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import { errorText } from "@/lib/errors";
import { ImageDetailBody } from "./imageLayout/ImageDetailBody";
import { ImageHeader } from "./imageLayout/ImageHeader";
import { useImageDetail } from "./imageLayout/useImageDetail";

const TABS = [
  { to: ".", label: "Overview", end: true },
  { to: "template", label: "Template", end: false },
  { to: "skills", label: "Skills", end: false },
  { to: "files", label: "Files", end: false },
];

export function ImageLayout({
  hostId,
  basePath = "/images",
}: {
  hostId?: string;
  basePath?: string;
}) {
  const { name = "", tag = "" } = useParams();
  const hostKey = hostId ?? "";
  return (
    <ImageDetail
      key={`${hostKey}:${name}:${tag}`}
      hostKey={hostKey}
      name={name}
      tag={tag}
      basePath={basePath}
    />
  );
}

function ImageDetail({
  hostKey,
  name,
  tag,
  basePath,
}: {
  hostKey: string;
  name: string;
  tag: string;
  basePath: string;
}) {
  const ref = `${name}:${tag}`;
  const navigate = useNavigate();
  const { detail, error, notice, onReadError, refresh } = useImageDetail(hostKey, ref);

  const remove = async () => {
    try {
      if (!detail) return;
      await detail.api.images.removeImage(ref);
      toast.success(`image ${ref} removed`);
      navigate(`${basePath}/${encodeURIComponent(name)}`);
    } catch (err) {
      toast.error(`remove failed: ${errorText(err)}`);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <nav aria-label="Image breadcrumbs" className="flex gap-2 border-b px-4 py-2 text-sm">
        <Link className="text-primary hover:underline" to={basePath}>
          Images
        </Link>
        <span>/</span>
        <Link
          className="text-primary hover:underline"
          to={`${basePath}/${encodeURIComponent(name)}`}
        >
          {name}
        </Link>
        <span>/</span>
        <span>{tag}</span>
      </nav>
      {notice && (
        <p role="status" className="px-4 py-2 text-sm text-muted-foreground">
          {notice}
        </p>
      )}
      <ImageHeader
        imageRef={ref}
        hostKey={hostKey}
        manifest={detail?.manifest}
        provenance={detail?.provenance}
        onRemove={() => void remove()}
      />
      <nav className="flex items-center gap-1 border-b px-2">
        {TABS.map((tabItem) => (
          <NavLink
            key={tabItem.to}
            to={tabItem.to}
            end={tabItem.end}
            className={({ isActive }) =>
              cn(
                "px-3 py-2 text-sm",
                isActive ? "border-b-2 border-primary font-medium" : "text-muted-foreground",
              )
            }
          >
            {tabItem.label}
          </NavLink>
        ))}
      </nav>
      <ImageDetailBody
        imageRef={ref}
        hostKey={hostKey}
        detail={detail}
        error={error}
        onReadError={onReadError}
        onRefresh={refresh}
      />
    </div>
  );
}
