import { StoryTestCases } from '../../../types/storyTypes';

export default function TestCases({ cases }: StoryTestCases) {
  return (
    <div
      id={'test_cases'}
      className={'proxima_nova flex flex-col gap-4 text-sm font-normal'}
    >
      {cases &&
        cases.length > 0 &&
        cases.map((test, index) => (
          <div key={index}>
            <span>
              {index + 1}. {test}
            </span>
          </div>
        ))}
    </div>
  );
}
