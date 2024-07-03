import React from 'react';
import {
  CommitItems,
  CommitLogsProps,
} from '../../../../../types/pullRequestsTypes';
import Image from 'next/image';
import imagePath from '@/app/imagePath';
import styles from '@/app/(programmer)/pull_request/pr.module.css';

export default function CommitLogs({ commits }: CommitLogsProps) {
  const sortAndGroupCommits = (commits: CommitItems[]) => {
    const sortedCommits = [...commits].sort(
      (a, b) => new Date(b.date).getTime() - new Date(a.date).getTime(),
    );
    const groupedCommits: { [key: string]: CommitItems[] } = {};
    sortedCommits.forEach((commit) => {
      if (!groupedCommits[commit.date]) {
        groupedCommits[commit.date] = [];
      }
      groupedCommits[commit.date].push(commit);
    });
    return groupedCommits;
  };

  const groupedCommits = sortAndGroupCommits(commits);

  return (
    <div id={'commit_logs'} className={'flex flex-col px-[12vw] py-5'}>
      {Object.keys(groupedCommits).map((date) => (
        <div key={date} className={'mb-5'}>
          <div
            id={'commit_date'}
            className={
              'color_666 mb-2 flex flex-row items-center gap-2 text-[13px] font-normal'
            }
          >
            <Image
              width={0}
              height={0}
              sizes={'100vw'}
              className={'size-5'}
              src={imagePath.commitsIconUnselected}
              alt={'commits_icon'}
            />
            Commits on {date}
          </div>

          <div id={'commits_in_accordance_to_date'} className={'px-4'}>
            {groupedCommits[date].map((commit, index) => (
              <div
                className={`${styles.commit_log_card_container} mb-3`}
                key={index}
              >
                <div className={'flex flex-col gap-2'}>
                  <div className={'text-base font-normal'}>{commit.title}</div>
                  <div
                    className={
                      'secondary_color flex flex-row items-center gap-2 text-[11px]'
                    }
                  >
                    <Image
                      width={0}
                      height={0}
                      sizes={'100vw'}
                      className={'size-4'}
                      src={imagePath.superagiLogoRound}
                      alt={'developer_logo'}
                    />
                    <span className={'font-semibold text-white'}>
                      {commit.commiter}
                    </span>
                    committed {commit.time}
                  </div>
                </div>

                <div
                  className={'flex flex-row items-center justify-center gap-2'}
                >
                  <div
                    className={'secondary_color space_mono text-sm font-normal'}
                  >
                    {commit.sha}
                  </div>

                  <Image
                    width={0}
                    height={0}
                    sizes={'100vw'}
                    className={'h-5 w-1'}
                    src={imagePath.verticalLine}
                    alt={'vertical_line'}
                  />

                  <Image
                    width={0}
                    height={0}
                    sizes={'100vw'}
                    className={'size-4'}
                    src={imagePath.codeIconUnselected}
                    alt={'code_block_icon'}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
