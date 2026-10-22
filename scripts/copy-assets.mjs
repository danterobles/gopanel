import { copyFile } from 'node:fs/promises';
await copyFile('node_modules/htmx.org/dist/htmx.min.js', 'web/static/htmx.min.js');
await copyFile('node_modules/htmx-ext-sse/dist/sse.js', 'web/static/sse.js');
await copyFile('node_modules/htmx.org/LICENSE', 'web/static/htmx.LICENSE');
await copyFile('node_modules/htmx-ext-sse/LICENSE', 'web/static/sse.LICENSE');
await copyFile('node_modules/tailwindcss/LICENSE', 'web/static/tailwind.LICENSE');
