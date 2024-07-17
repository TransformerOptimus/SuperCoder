'use client';
import StoryDetails from '@/components/StoryComponents/StoryDetails';
import { StoryDetailsWorkbenchProps } from '../../../types/workbenchTypes';

export default function StoryDetailsWorkbench({
  id,
}: StoryDetailsWorkbenchProps) {
  return (
    <div id={'story_details'}>
      <StoryDetails story_id={id} id={'workbench'} />
    </div>
  );
}
