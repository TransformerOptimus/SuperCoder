import { useState } from 'react';
import { Alert } from 'antd';

interface QuestionBannerProps {
  question: string;
  options?: string[];
  onAnswer: (answer: string) => void;
  onSkip: () => void;
  disabled?: boolean;
}

export default function QuestionBanner({ question, options, onAnswer, onSkip, disabled }: QuestionBannerProps) {
  const [selectedOption, setSelectedOption] = useState<string | null>(null);
  const [customText, setCustomText] = useState('');

  const handleSubmit = () => {
    const answer = selectedOption || customText.trim();
    if (answer) {
      onAnswer(answer);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  return (
    <div className="mx-1 my-2">
      <Alert
        type="info"
        showIcon={false}
        message="Agent needs clarification"
        description={
          <div className="flex flex-col gap-3 mt-1">
            <p className="text-[13px] text-[var(--text-primary)] m-0 leading-relaxed">
              {question}
            </p>

            {options && options.length > 0 && (
              <div className="flex flex-col gap-1">
                {options.map((opt) => (
                  <button
                    key={opt}
                    type="button"
                    onClick={() => { setSelectedOption(opt); setCustomText(''); }}
                    disabled={disabled}
                    className={`flex items-center gap-2.5 px-3 py-2 rounded-lg border text-left text-[13px] transition-colors ${
                      selectedOption === opt
                        ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-300'
                        : 'border-[var(--border-color-8)] text-[var(--text-primary)] hover:bg-[var(--hover-bg)]'
                    }`}
                  >
                    <span className={`w-3.5 h-3.5 rounded-full border-2 shrink-0 flex items-center justify-center ${
                      selectedOption === opt
                        ? 'border-blue-500'
                        : 'border-[var(--white-opacity-20)]'
                    }`}>
                      {selectedOption === opt && (
                        <span className="w-1.5 h-1.5 rounded-full bg-blue-500" />
                      )}
                    </span>
                    {opt}
                  </button>
                ))}
              </div>
            )}

            <input
              className="input_small"
              type="text"
              placeholder={options?.length ? 'Or type a custom answer...' : 'Type your answer...'}
              value={customText}
              onChange={(e) => { setCustomText(e.target.value); setSelectedOption(null); }}
              onKeyDown={handleKeyDown}
              disabled={disabled}
            />

            <div className="flex gap-2 justify-end">
              <button className="secondary_small" onClick={onSkip} disabled={disabled}>
                Skip
              </button>
              <button
                className="primary_small"
                onClick={handleSubmit}
                disabled={disabled || (!selectedOption && !customText.trim())}
              >
                Answer
              </button>
            </div>
          </div>
        }
      />
    </div>
  );
}
