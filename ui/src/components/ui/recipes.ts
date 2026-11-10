/**
 * Recipes: the class lists that more than one screen repeats. A recipe is a
 * string, not a component, because the same look sits on different elements
 * (a `<span>`, a `<label>`, an `<Input>`, a `<li>`) and a caller adds its own
 * layout with `cn(LABEL, "ml-auto")`.
 *
 * Nothing here decides what a thing means. A recipe names only tokens of
 * src/index.css, so a change of a token or of a recipe reaches every screen.
 * The story Foundations/Recipes shows each one.
 */

/** Every section label and field label reads the same. */
export const LABEL = "text-caption font-medium tracking-wide text-muted-foreground";
/** A field is a fill, not a box: 30px of `--muted` at the control radius, no border. */
export const FIELD =
  "h-7.5 w-full min-w-0 rounded-control border-0 bg-muted px-2.5 text-body md:text-body text-foreground shadow-none outline-none focus-visible:ring-2 focus-visible:ring-ring";
/** A field carrying an identifier rather than prose. */
export const FIELD_MONO = "font-mono text-caption md:text-caption";
/** Keys, times, durations and digests line up in one column. */
export const MONO = "font-mono text-caption tabular-nums";
/** A row of a list inside a panel: dependencies, history, visits. */
export const ROW =
  "-mx-1.5 flex min-h-7.5 items-center gap-2.5 rounded-control px-2 hover:bg-muted";
/** The counter that follows a section label. */
export const COUNT = "font-mono text-caption tabular-nums text-muted-foreground opacity-75";
/** A quiet 24px action sitting on the right of a section header. */
export const QUIET_ACTION =
  "h-6 rounded-control px-2.5 text-ui font-normal text-muted-foreground hover:bg-accent hover:text-foreground";
/** The one empty-state voice: a single quiet line where the list would be. */
export const EMPTY = "text-ui text-muted-foreground opacity-80";
/** The banner fills. Translucent, so they sit on any surface. */
export const DANGER_FILL = "bg-destructive-soft text-destructive";
export const PRIMARY_FILL = "bg-primary-soft text-primary";
export const WARNING_FILL = "bg-warning-soft text-warning";
export const SUCCESS_FILL = "bg-success-soft text-success";
/** The content island: a white surface on the metal, under the one high shadow. */
export const ISLAND = "rounded-panel bg-card shadow-lift";
/** A cell of a dense data table: every table keeps the same row height. */
export const CELL = "px-3 py-1.5 whitespace-nowrap";
/** The first column stays at the left edge while the table scrolls sideways. */
export const STICKY_CELL = "sticky left-0 z-sticky bg-card";
