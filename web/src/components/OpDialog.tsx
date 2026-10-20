// The form in its dialog, with the accounts and assets it offers.

import { get, useLoad, type Account, type AssetRef, type Op } from "../api";
import { KIND } from "../ops";
import { Dialog } from "./Dialog";
import { OpDetails, OpForm } from "./OpForm";
import { Loading } from "./State";

/** asset: a new operation's asset, when it is added from that asset's page. */
export function OpDialog({ op, asset, onClose, onSaved }: { op?: Op; asset?: string; onClose: () => void; onSaved: () => void }) {
  const accounts = useLoad("accounts", (s) => get<Account[]>("accounts", s));
  const used = useLoad("assets", (s) => get<AssetRef[]>("assets", s));
  const title = !op ? "Ajouter une opération" : op.editable ? "Modifier l'opération" : KIND[op.kind];
  return (
    <Dialog title={title} onClose={onClose}>
      {op && !op.editable ? (
        <OpDetails op={op} onClose={onClose} />
      ) : accounts.data && used.data ? (
        <OpForm op={op} asset={asset} accounts={accounts.data} used={used.data} onSaved={onSaved} onCancel={onClose} />
      ) : accounts.error ? (
        <p className="form-error">{accounts.error}</p>
      ) : (
        <Loading small />
      )}
    </Dialog>
  );
}
