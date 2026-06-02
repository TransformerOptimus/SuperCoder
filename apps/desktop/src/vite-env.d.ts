/// <reference types="vite/client" />

// refractor@3 ships no type declarations for its subpath modules.
declare module "refractor/core" {
  const refractor: {
    register: (lang: unknown) => void;
    highlight: (value: string, language: string) => unknown;
    registered: (language: string) => boolean;
  };
  export default refractor;
}
declare module "refractor/lang/*" {
  const lang: unknown;
  export default lang;
}
