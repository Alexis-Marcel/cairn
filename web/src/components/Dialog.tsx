// A modal: the browser's own <dialog>, so focus, Escape and the backdrop
// behave as they should. A sheet from the bottom on phones.

import { useEffect, useRef } from "react";

export function Dialog({
  title,
  onClose,
  children,
  wide,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  wide?: boolean;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (d && !d.open) {
      d.showModal();
      // The dialog itself, not its close button, has the focus first.
      d.focus();
    }
    document.documentElement.classList.add("modal-open");
    return () => document.documentElement.classList.remove("modal-open");
  }, []);
  return (
    <dialog
      ref={ref}
      tabIndex={-1}
      className={wide ? "dialog wide" : "dialog"}
      aria-labelledby="dialog-title"
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      onMouseDown={(e) => {
        // A click on the backdrop (the dialog itself, outside its card).
        if (e.target === ref.current) onClose();
      }}
    >
      <div className="dialog-card">
        <header className="dialog-head">
          <h2 id="dialog-title">{title}</h2>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Fermer">
            <svg viewBox="0 0 16 16" aria-hidden>
              <path d="M4 4l8 8M12 4l-8 8" />
            </svg>
          </button>
        </header>
        {children}
      </div>
    </dialog>
  );
}
