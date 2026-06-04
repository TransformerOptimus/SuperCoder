## Getting Started

First, run the development server:

```bash
yarn install

npx husky install

npx husky add .husky/pre-commit "npx lint-staged"
```

then

```bash
yarn dev
# or
pnpm dev
# or
bun dev
```

Open [http://localhost:3000](;http://localhost:3000) with your browser to see the result.
