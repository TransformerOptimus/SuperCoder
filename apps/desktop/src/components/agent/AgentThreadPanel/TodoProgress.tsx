import { Progress, Collapse } from 'antd';
import { Check, Loader2, Circle } from 'lucide-react';
import type { TodoItem } from '@/types/agentContract';

interface TodoProgressProps {
  todos: TodoItem[];
}

export default function TodoProgress({ todos }: TodoProgressProps) {
  const completed = todos.filter((t) => t.status === 'completed').length;
  const total = todos.length;
  const percent = total > 0 ? Math.round((completed / total) * 100) : 0;

  if (total === 0) return null;

  return (
    <div className="mx-1 my-2">
      <Collapse
        ghost
        size="small"
        defaultActiveKey={['progress']}
        items={[
          {
            key: 'progress',
            label: (
              <div className="flex items-center gap-3 flex-1 min-w-0">
                <span className="text-xs font-semibold text-[var(--text-secondary)] whitespace-nowrap">
                  Agent Progress ({completed}/{total})
                </span>
                <Progress
                  percent={percent}
                  size="small"
                  showInfo={false}
                  className="flex-1 min-w-0"
                />
              </div>
            ),
            children: (
              <ul className="list-none p-0 m-0 flex flex-col gap-0.5">
                {todos.map((todo) => (
                  <li key={todo.id} className="flex items-start gap-2 text-xs leading-relaxed py-0.5">
                    {todo.status === 'completed' && (
                      <Check className="w-3.5 h-3.5 shrink-0 mt-0.5 text-green-500" />
                    )}
                    {todo.status === 'in_progress' && (
                      <Loader2 className="w-3.5 h-3.5 shrink-0 mt-0.5 text-blue-500 animate-spin" />
                    )}
                    {todo.status === 'pending' && (
                      <Circle className="w-3.5 h-3.5 shrink-0 mt-0.5 text-[var(--text-secondary)] opacity-40" />
                    )}
                    <span
                      className={todo.status === 'completed' ? 'text-[var(--text-secondary)] line-through' : 'text-[var(--text-primary)]'}
                    >
                      {todo.content}
                    </span>
                  </li>
                ))}
              </ul>
            ),
          },
        ]}
      />
    </div>
  );
}
