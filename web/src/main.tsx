import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './styles/global.css';
import { LocaleProvider } from './contexts/LocaleContext';

const root = document.getElementById('root');
if (!root) {
  throw new Error('root element not found');
}

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <LocaleProvider defaultLang="zh">
      <App />
    </LocaleProvider>
  </React.StrictMode>
);
