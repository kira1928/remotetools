import { useEffect, useRef } from 'react';

interface UseSSEOptions {
  onError?: (event: MessageEvent<any> | Event) => void;
  enabled?: boolean;
}

/**
 * useSSE 建立 Server-Sent Events 连接，并在组件卸载时自动清理。
 */
export function useSSE(url: string, onMessage: (event: MessageEvent<any>) => void, options: UseSSEOptions = {}) {
  const { onError, enabled = true } = options;
  const handlerRef = useRef(onMessage);
  const errorRef = useRef(onError);

  handlerRef.current = onMessage;
  errorRef.current = onError;

  useEffect(() => {
    if (!enabled) {
      return undefined;
    }
    const es = new EventSource(url);
    es.onmessage = (event) => handlerRef.current(event);
    if (errorRef.current) {
      es.onerror = (event) => {
        errorRef.current?.(event);
      };
    }
    return () => {
      es.close();
    };
  }, [url, enabled]);
}
