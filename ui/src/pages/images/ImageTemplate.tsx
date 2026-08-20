import { useEffect,useState } from "react";
import { imageTemplateGet,type ImagePromptTemplate } from "@/lib/api";
import { useImageContext } from "@/components/ImageLayout";

export default function ImageTemplate(){
  const {ref,hostKey}=useImageContext();const [template,setTemplate]=useState<ImagePromptTemplate|null>(null);const [error,setError]=useState("");
  useEffect(()=>{let alive=true;void imageTemplateGet(ref).then(value=>{if(alive){setTemplate(value);setError("")}}).catch(cause=>{if(alive)setError(String(cause))});return()=>{alive=false}},[hostKey,ref]);
  if(error)return <p className="text-sm text-destructive">{error}</p>;if(!template)return <p className="text-sm text-muted-foreground">Loading…</p>;
  return <div className="space-y-3"><div className="font-mono text-xs text-muted-foreground">template sha256 {template.sha256}</div>
    <ol className="space-y-2">{template.entries.map((entry,index)=><li key={`${index}-${entry.archive_path??entry.runtime}`} className="rounded border p-3 text-sm">
      <div className="flex gap-3"><span className="w-8 font-mono text-muted-foreground">{index}</span><strong>{entry.kind}</strong>{entry.runtime&&<code>{entry.runtime}</code>}</div>
      {entry.source&&<div className="mt-2 break-all font-mono text-xs">{entry.source}</div>}
      {entry.category&&<div className="text-xs text-muted-foreground">{entry.category} · {entry.size??0} bytes · {entry.sha256?.slice(0,16)}</div>}
    </li>)}</ol>
    {template.entries.length===0&&<p className="text-sm text-muted-foreground">This image has an empty prompt template.</p>}
  </div>;
}
