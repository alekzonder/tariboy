import { useEffect, useState } from "react";
import { Link, NavLink, useMatch, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { createApi } from "@/lib/api";
import { resolveDaemon } from "@/lib/daemons";
import { cn } from "@/lib/utils";
import { errorText } from "@/lib/errors";
import BuiltImages from "@/pages/images/BuiltImages";
import { groupBuiltImages } from "@/pages/images/built/builtImages";
import { ApiProvider } from "./ApiProvider";
import { ImageDetailBody } from "./imageLayout/ImageDetailBody";
import { ImageHeader } from "./imageLayout/ImageHeader";
import { useImageDetail } from "./imageLayout/useImageDetail";

const IMAGES_PATH = "/library/images";

const TABS = [
  { path: "", label: "Overview" },
  { path: "/template", label: "Template" },
  { path: "/skills", label: "Skills" },
  { path: "/files", label: "Files" },
];

const namePath = (name: string) => `${IMAGES_PATH}/${encodeURIComponent(name)}`;

/** One image of the Library: `/library/images/:name` shows its newest tag,
 *  `/library/images/:name/:tag/*` that tag. */
export function ImageLayout({ hostId }: { hostId?: string }) {
  const { name = "", tag } = useParams();
  const hostKey = hostId ?? "";
  if (tag === undefined) {
    return <NewestImageDetail key={`${hostKey}:${name}`} hostKey={hostKey} name={name} />;
  }
  return <ImageDetail key={`${hostKey}:${name}:${tag}`} hostKey={hostKey} name={name} tag={tag} />;
}

/** The newest tag of `name` on the host, or null when it has none. */
function useNewestTag(hostKey: string, name: string) {
  const [state, setState] = useState<{ tag?: string | null; error: string }>({ error: "" });
  useEffect(() => {
    let alive = true;
    void (async () => {
      const target = await resolveDaemon(hostKey);
      if (hostKey && !target) throw new Error(`host ${hostKey} is not available`);
      const result = await createApi(target).images.listImages();
      const group = groupBuiltImages(result.images ?? []).find((entry) => entry.name === name);
      if (alive) setState({ tag: group?.tags[0]?.tag ?? null, error: "" });
    })().catch((cause) => {
      if (alive) setState({ error: errorText(cause) });
    });
    return () => {
      alive = false;
    };
  }, [hostKey, name]);
  return state;
}

function NewestImageDetail({ hostKey, name }: { hostKey: string; name: string }) {
  const { tag, error } = useNewestTag(hostKey, name);
  if (tag) return <ImageDetail hostKey={hostKey} name={name} tag={tag} nameRoute />;
  return (
    <div className="flex h-full flex-col">
      <Breadcrumbs name={name} />
      <div className="p-4 text-sm">
        {error ? (
          <p role="alert" className="text-destructive">
            {error}
          </p>
        ) : tag === null ? (
          <p className="text-muted-foreground">
            No tags for this image on this host.{" "}
            <Link className="text-primary hover:underline" to={IMAGES_PATH}>
              Back to Images
            </Link>
          </p>
        ) : (
          <p className="text-muted-foreground">Loading…</p>
        )}
      </div>
    </div>
  );
}

function Breadcrumbs({ name, tag }: { name: string; tag?: string }) {
  return (
    <nav aria-label="Image breadcrumbs" className="flex gap-2 border-b px-4 py-2 text-sm">
      <Link className="text-primary hover:underline" to={IMAGES_PATH}>
        Images
      </Link>
      <span>/</span>
      {tag === undefined ? (
        <span>{name}</span>
      ) : (
        <>
          <Link className="text-primary hover:underline" to={namePath(name)}>
            {name}
          </Link>
          <span>/</span>
          <span>{tag}</span>
        </>
      )}
    </nav>
  );
}

function ImageDetail({
  hostKey,
  name,
  tag,
  nameRoute = false,
}: {
  hostKey: string;
  name: string;
  tag: string;
  /** Opened at `/library/images/:name`: the Overview link stays there. */
  nameRoute?: boolean;
}) {
  const ref = `${name}:${tag}`;
  const navigate = useNavigate();
  const { detail, error, notice, onReadError, refresh } = useImageDetail(hostKey, ref);
  const tagPath = `${namePath(name)}/${encodeURIComponent(tag)}`;
  const onTab = useMatch(`${IMAGES_PATH}/:name/:tag/:tab`) !== null;

  const remove = async () => {
    try {
      if (!detail) return;
      await detail.api.images.removeImage(ref);
      toast.success(`image ${ref} removed`);
      navigate(nameRoute ? IMAGES_PATH : namePath(name));
    } catch (err) {
      toast.error(`remove failed: ${errorText(err)}`);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <Breadcrumbs name={name} tag={nameRoute ? undefined : tag} />
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
            key={tabItem.label}
            to={tabItem.path === "" && nameRoute ? namePath(name) : `${tagPath}${tabItem.path}`}
            end={tabItem.path === ""}
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
        footer={
          !onTab &&
          detail && (
            <ApiProvider client={detail.api}>
              <BuiltImages hostId={hostKey} imageName={name} currentTag={tag} />
            </ApiProvider>
          )
        }
      />
    </div>
  );
}
