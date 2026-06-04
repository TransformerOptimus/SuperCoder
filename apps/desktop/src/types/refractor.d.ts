// refractor v3 ships no TypeScript types and has no @types package.
// Minimal declarations for the subset we use (core + per-language grammars).
declare module 'refractor/core' {
  interface RefractorRoot {
    type: 'root';
    children: unknown[];
  }
  interface Refractor {
    register(grammar: unknown): void;
    registered(name: string): boolean;
    highlight(value: string, name: string): RefractorRoot;
  }
  const refractor: Refractor;
  export default refractor;
}

declare module 'refractor/lang/*.js' {
  const grammar: unknown;
  export default grammar;
}
