import { PullRequestsProvider } from '@/context/PullRequests';

export default function PullRequestLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <section>
      <PullRequestsProvider>{children}</PullRequestsProvider>
    </section>
  );
}
