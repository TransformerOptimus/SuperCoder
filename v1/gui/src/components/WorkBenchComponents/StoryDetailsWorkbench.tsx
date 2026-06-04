'use client';
import StoryDetails from '@/components/StoryComponents/StoryDetails';
import { StoryDetailsWorkbenchProps } from '../../../types/workbenchTypes';

export default function StoryDetailsWorkbench({
  id,
}: StoryDetailsWorkbenchProps) {
  return (
    <div id={'story_details'} className={'p-4'}>
      <StoryDetails story_id={id} id={'workbench'} />
    </div>
  );
}
