import { WorkbenchProvider } from '@/context/Workbench';

export default function BoardLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <section>
      <WorkbenchProvider>{children}</WorkbenchProvider>
    </section>
  );
}
