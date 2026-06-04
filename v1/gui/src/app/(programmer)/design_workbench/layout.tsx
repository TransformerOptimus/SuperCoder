export default function DesignWorkbenchLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <section>
      {/*<WorkbenchProvider>*/}
      {children}
      {/*</WorkbenchProvider>*/}
    </section>
  );
}
