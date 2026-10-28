// subscribeResume calls listener when the page becomes visible again or the
// browser comes back online. A WebView can keep a socket "open" after its
// connection died (a dropped port forward on Android), so live channels redial
// and reads refetch on resume instead of trusting the old connection.
export function subscribeResume(listener: () => void): () => void {
  const onVisibility = () => {
    if (document.visibilityState === "visible") listener();
  };
  document.addEventListener("visibilitychange", onVisibility);
  window.addEventListener("online", listener);
  return () => {
    document.removeEventListener("visibilitychange", onVisibility);
    window.removeEventListener("online", listener);
  };
}
