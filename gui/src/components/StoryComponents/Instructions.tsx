import { StoryInstructions } from '../../../types/storyTypes';

export default function Instructions({ instructions }: StoryInstructions) {
  return (
    <div
      id={'instructions'}
      className={'proxima_nova flex flex-col gap-4 text-sm font-normal'}
    >
      {instructions &&
        instructions.length > 0 &&
        instructions.map((instruction, index) => (
          <div key={index}>
            <span>{instruction}</span>
          </div>
        ))}
    </div>
  );
}
