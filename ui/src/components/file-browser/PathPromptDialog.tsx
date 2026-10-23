import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";

export function PathPromptDialog({
  open, title, label, value, onValueChange, submitLabel, onSubmit, onOpenChange,
}: {
  open: boolean;
  title: string;
  label: string;
  value: string;
  onValueChange: (value: string) => void;
  submitLabel: string;
  onSubmit: (value: string) => void;
  onOpenChange: (open: boolean) => void;
}) {
  const submit = () => {
    const v = value.trim();
    if (!v) return;
    onSubmit(v);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">{label}</span>
          <Input
            autoFocus
            value={value}
            onChange={(e) => onValueChange(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); submit(); } }}
          />
        </label>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={submit} disabled={!value.trim()}>{submitLabel}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
