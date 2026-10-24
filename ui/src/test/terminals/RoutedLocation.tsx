import { useLocation } from "react-router-dom";

/** The router's current path and query, for tests to assert navigation on. */
export function RoutedLocation() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}
