// Tout, Crypto or Bourse: what every figure covers. One choice for the
// whole app, kept in this browser.

import type { Scope } from "../api";
import { Segmented } from "./Segmented";

const OPTIONS: { value: Scope; label: string }[] = [
  { value: "all", label: "Tout" },
  { value: "crypto", label: "Crypto" },
  { value: "securities", label: "Bourse" },
];

export function ScopeSwitch({ scope, onChange }: { scope: Scope; onChange: (s: Scope) => void }) {
  return <Segmented label="Portefeuille" className="toggle small scope" options={OPTIONS} value={scope} onChange={onChange} />;
}
