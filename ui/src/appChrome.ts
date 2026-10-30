const CONSOLE =
  /^\/(?:$|tasks$|inbox$|activity$|agents\/|servers\/|library(?:$|\/)|settings(?:$|\/))/;

/** Whether the path is a console page: the sidebar, its toggle and the
 *  global search belong there. */
export function sidebarRoute(pathname: string): boolean {
  return CONSOLE.test(pathname);
}
