import { useId, useState } from 'react';
import type { InputHTMLAttributes, ReactNode } from 'react';
import { Icon } from './ui';

type InputProps = InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  hint?: string;
  error?: string;
};

function InputFrame({
  label,
  hint,
  error,
  id,
  children,
}: {
  label: string;
  hint?: string;
  error?: string;
  id: string;
  children: ReactNode;
}) {
  return (
    <div className="field">
      <label className="field__label" htmlFor={id}>
        {label}
      </label>
      {children}
      {hint && (
        <span className="field__hint" id={id + '-hint'}>
          {hint}
        </span>
      )}
      {error && (
        <span className="field__error" id={id + '-error'} role="alert">
          {error}
        </span>
      )}
    </div>
  );
}

export function InputField({ label, hint, error, id: suppliedId, ...props }: InputProps) {
  const generatedId = useId();
  const id = suppliedId || generatedId;
  const describedBy =
    [hint && id + '-hint', error && id + '-error', props['aria-describedby']]
      .filter(Boolean)
      .join(' ') || undefined;
  return (
    <InputFrame label={label} hint={hint} error={error} id={id}>
      <input
        {...props}
        id={id}
        aria-describedby={describedBy}
        aria-invalid={!!error || undefined}
      />
    </InputFrame>
  );
}

export function PasswordField({
  label = 'Пароль',
  hint,
  error,
  id: suppliedId,
  ...props
}: Omit<InputProps, 'label' | 'type'> & { label?: string }) {
  const generatedId = useId();
  const id = suppliedId || generatedId;
  const [visible, setVisible] = useState(false);
  return (
    <InputFrame label={label} hint={hint} error={error} id={id}>
      <div className="password-input">
        <input
          {...props}
          type={visible ? 'text' : 'password'}
          id={id}
          aria-describedby={
            [hint && id + '-hint', error && id + '-error'].filter(Boolean).join(' ') || undefined
          }
          aria-invalid={!!error || undefined}
        />
        <button
          type="button"
          aria-label={visible ? 'Скрыть пароль' : 'Показать пароль'}
          aria-pressed={visible}
          onClick={() => setVisible(!visible)}
          disabled={props.disabled}
        >
          <Icon name="eye" size={18} />
        </button>
      </div>
    </InputFrame>
  );
}
