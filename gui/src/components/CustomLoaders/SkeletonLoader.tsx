import React from 'react';
import Skeleton from 'react-loading-skeleton';
import 'react-loading-skeleton/dist/skeleton.css';
import project from '@/app/projects/projects.module.css';
import { SkeletonTypes } from '@/app/constants/SkeletonConstants';

const baseColor = 'rgba(255, 255, 255, 0.06)';
const highlightColor = 'rgba(255, 255, 255, 0.10)';

interface SkeletonLoaderProps {
  type: string;
}

const ProjectSkeleton: React.FC = () => {
  return (
    <div>
      <Skeleton
        height={30}
        baseColor={baseColor}
        highlightColor={highlightColor}
        enableAnimation={false}
        className={'custom-skeleton'}
      />
      <div className={'mt-6 grid grid-cols-12 gap-3'}>
        {[...Array(7)].map((_, index) => (
          <div
            key={index}
            className={`${project.project_container} card_container col-span-6 flex w-full flex-col justify-between rounded-lg p-3`}
          >
            <div className="flex flex-col">
              <Skeleton
                height={20}
                width={150}
                baseColor={baseColor}
                highlightColor={highlightColor}
                enableAnimation={false}
                className={'custom-skeleton'}
              />
              <Skeleton
                height={15}
                width={'94%'}
                baseColor={baseColor}
                highlightColor={highlightColor}
                enableAnimation={false}
                className={'custom-skeleton mt-2'}
              />
              <Skeleton
                height={15}
                width={'74%'}
                baseColor={baseColor}
                highlightColor={highlightColor}
                enableAnimation={false}
                className={'custom-skeleton'}
              />
            </div>
            <Skeleton
              height={15}
              width={'40%'}
              baseColor={baseColor}
              highlightColor={highlightColor}
              enableAnimation={false}
              className={'custom-skeleton'}
            />
          </div>
        ))}
      </div>
    </div>
  );
};

const BoardSkeleton: React.FC = () => {
  return (
    <div className={'flex flex-col gap-2 p-4'}>
      <div className={'flex w-full flex-row gap-2'}>
        <div className={'grow'}>
          <Skeleton
            height={34}
            baseColor={baseColor}
            highlightColor={highlightColor}
            enableAnimation={false}
            className={'custom-skeleton'}
          />
        </div>
        <Skeleton
          height={34}
          width={90}
          baseColor={baseColor}
          highlightColor={highlightColor}
          enableAnimation={false}
          className={'custom-skeleton'}
        />
      </div>
      <div className={'grid size-full grid-cols-12 gap-2'}>
        {[...Array(4)].map((_, index) => (
          <div
            key={index}
            className={'card_container col-span-3 flex flex-col gap-2 p-2'}
          >
            <div className="mb-1 mt-2 flex flex-col gap-2">
              <Skeleton
                height={14}
                width={50}
                baseColor={baseColor}
                highlightColor={highlightColor}
                enableAnimation={false}
                className={'custom-skeleton'}
              />
              {index % 2 !== 0 && (
                <div className={'task_container_skeleton'}>
                  <Skeleton
                    height={14}
                    width={'100%'}
                    baseColor={baseColor}
                    highlightColor={highlightColor}
                    enableAnimation={false}
                    className={'custom-skeleton'}
                  />
                  <Skeleton
                    height={14}
                    width={'30%'}
                    baseColor={baseColor}
                    highlightColor={highlightColor}
                    enableAnimation={false}
                    className={'custom-skeleton'}
                  />
                </div>
              )}
              <div className={'task_container_skeleton'}>
                <Skeleton
                  height={14}
                  width={'100%'}
                  baseColor={baseColor}
                  highlightColor={highlightColor}
                  enableAnimation={false}
                  className={'custom-skeleton'}
                  count={3}
                />
                <Skeleton
                  height={14}
                  width={'30%'}
                  baseColor={baseColor}
                  highlightColor={highlightColor}
                  enableAnimation={false}
                  className={'custom-skeleton'}
                />
              </div>

              <div className={'task_container_skeleton'}>
                <Skeleton
                  height={14}
                  width={'100%'}
                  baseColor={baseColor}
                  highlightColor={highlightColor}
                  enableAnimation={false}
                  className={'custom-skeleton'}
                />
                <Skeleton
                  height={14}
                  width={'30%'}
                  baseColor={baseColor}
                  highlightColor={highlightColor}
                  enableAnimation={false}
                  className={'custom-skeleton'}
                />
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

const SkeletonLoader: React.FC<SkeletonLoaderProps> = ({ type }) => {
  return (
    <>
      {type === SkeletonTypes.PROJECT && <ProjectSkeleton />}
      {type === SkeletonTypes.BOARD && <BoardSkeleton />}
    </>
  );
};

export default SkeletonLoader;
