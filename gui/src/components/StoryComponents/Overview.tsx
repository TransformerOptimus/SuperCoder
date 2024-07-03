import { StoryOverview } from '../../../types/storyTypes';

export default function Overview({ overview }: StoryOverview) {
  return (
    overview && (
        <div
            id={'overview'}
            className={'proxima_nova flex flex-col gap-8 text-sm font-normal'}
        >
            <div id={'name'} className={'flex flex-col'}>
                <span className={'secondary_color'}>NAME</span>
                <span>{overview.name}</span>
            </div><br/>
            <div id={'description'} className={'flex flex-col'}>
                <span className={'secondary_color'}>DESCRIPTION</span>
                <span>{overview.description}</span>
            </div>
        </div>
    )
  );
}
