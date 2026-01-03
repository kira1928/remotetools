import { Fragment, useMemo } from 'react';
import type { ReactNode } from 'react';

type JsonPrimitive = string | number | boolean | null;

type JsonValue = JsonPrimitive | JsonValue[] | { [key: string]: JsonValue };

function isUrl(value: string): boolean {
  return /^https?:\/\//i.test(value.trim());
}

function indent(depth: number): string {
  return '  '.repeat(depth);
}

function renderValue(value: JsonValue, depth: number): ReactNode {
  if (Array.isArray(value)) {
    if (value.length === 0) {
      return '[]';
    }
    return (
      <>
        {'['}
        {'\n'}
        {value.map((item, index) => (
          <Fragment key={index}>
            {indent(depth + 1)}
            {renderValue(item, depth + 1)}
            {index < value.length - 1 ? ',' : ''}
            {'\n'}
          </Fragment>
        ))}
        {indent(depth)}
        {']'}
      </>
    );
  }

  if (value && typeof value === 'object') {
    const entries = Object.entries(value);
    if (entries.length === 0) {
      return '{}';
    }
    return (
      <>
        {'{'}
        {'\n'}
        {entries.map(([key, val], idx) => (
          <Fragment key={key}>
            {indent(depth + 1)}
            <span className="json-key">"{key}"</span>
            {': '}
            {renderValue(val, depth + 1)}
            {idx < entries.length - 1 ? ',' : ''}
            {'\n'}
          </Fragment>
        ))}
        {indent(depth)}
        {'}'}
      </>
    );
  }

  if (typeof value === 'string') {
    return (
      <span className="json-string">
        "
        {isUrl(value) ? (
          <a className="json-link" href={value} target="_blank" rel="noopener noreferrer">
            {value}
          </a>
        ) : (
          value
        )}
        "
      </span>
    );
  }

  if (typeof value === 'number') {
    return <span className="json-number">{value}</span>;
  }

  if (typeof value === 'boolean') {
    return <span className="json-boolean">{value.toString()}</span>;
  }

  return <span className="json-null">null</span>;
}

export function JsonPreview({ json }: { json: string }) {
  const { parsed, failed } = useMemo(() => {
    if (!json) {
      return { parsed: undefined as JsonValue | undefined, failed: false };
    }
    try {
      const data = JSON.parse(json) as JsonValue;
      return { parsed: data, failed: false };
    } catch (error) {
      console.error('metadata parse failed', error);
      return { parsed: undefined as JsonValue | undefined, failed: true };
    }
  }, [json]);

  if (!json) {
    return null;
  }

  if (failed || parsed === undefined) {
    return (
      <pre className="json-preview json-preview-error">
        {json}
      </pre>
    );
  }

  return (
    <pre className="json-preview">
      {renderValue(parsed, 0)}
    </pre>
  );
}
