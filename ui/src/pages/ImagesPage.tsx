import { targetFor } from "@/lib/terminalsHost";
import ImageBuildFromDirectory from "./images/ImageBuildFromDirectory";
import BuiltImages from "./images/BuiltImages";

export default function ImagesPage({ hostId = "", basePath = "/images" }: {
  hostId?: string;
  basePath?: string;
}){
  return <div className="h-full min-h-0 space-y-4 overflow-y-auto p-6">
    <div><h1 className="text-lg font-semibold">Images</h1><p className="text-sm text-muted-foreground">Build plugin and prompt artifacts, inspect their exact template, and assign them to agents.</p></div>
    <ImageBuildFromDirectory key={hostId} target={targetFor(hostId)}/>
    <BuiltImages key={hostId} hostId={hostId} basePath={basePath}/>
  </div>;
}
