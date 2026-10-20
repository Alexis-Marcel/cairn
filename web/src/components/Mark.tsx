// Cairn's mark: three stones, stacked.

export function Mark() {
  return (
    <svg className="mark" viewBox="0 0 32 32" aria-hidden>
      <ellipse cx="16" cy="25.2" rx="12" ry="3.6" />
      <ellipse cx="14.6" cy="18.4" rx="8.4" ry="3.1" transform="rotate(-4 14.6 18.4)" />
      <ellipse cx="17.2" cy="12.2" rx="5.2" ry="2.6" transform="rotate(6 17.2 12.2)" />
    </svg>
  );
}
