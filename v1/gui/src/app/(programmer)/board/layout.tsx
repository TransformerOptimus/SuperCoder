import { BoardProvider } from '@/context/Boards';

export default function BoardLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <section>
      <BoardProvider>{children}</BoardProvider>
    </section>
  );
}
