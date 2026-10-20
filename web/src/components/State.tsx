// What a page shows while it loads, or when Fundarr did not answer.

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

export function Loading() {
  return <div className="loading" aria-busy="true" aria-label="Chargement" />;
}
