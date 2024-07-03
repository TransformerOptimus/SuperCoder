import { BoardProvider } from '@/context/Boards';

export default function CodeLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <section>
      <BoardProvider>{children}</BoardProvider>
    </section>
  );
}
