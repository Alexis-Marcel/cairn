// What a page shows while it loads, or when something did not answer.

export function Failed({ error, retry }: { error: string; retry: () => void }) {
  return (
    <div className="failed" role="alert">
      <p>{error}</p>
      <button type="button" className="btn" onClick={retry}>
        Réessayer
      </button>
    </div>
  );
}

export function Loading({ small }: { small?: boolean }) {
  return <div className={small ? "loading small" : "loading"} aria-busy="true" aria-label="Chargement" />;
}
