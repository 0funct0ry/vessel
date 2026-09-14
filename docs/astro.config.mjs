// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Vessel docs are published alongside the marketing site under the same
// GitHub Pages deployment, at /vessel/ (see .github/workflows/pages.yml).
export default defineConfig({
  site: 'https://0funct0ry.github.io',
  base: '/vessel/',
  integrations: [
    starlight({
      title: 'Vessel',
      description: 'A single-binary web UI for the Docker containers on one host.',
      customCss: ['./src/styles/custom.css'],
      social: {
        github: 'https://github.com/0funct0ry/vessel',
      },
      head: [
        {
          tag: 'link',
          attrs: { rel: 'preconnect', href: 'https://fonts.gstatic.com' },
        },
      ],
      editLink: {
        baseUrl: 'https://github.com/0funct0ry/vessel/edit/main/docs/',
      },
      pagination: true,
      sidebar: [
        {
          label: 'Start here',
          items: [
            { label: 'Introduction', slug: 'introduction' },
            { label: 'Install', slug: 'install' },
            { label: 'Quick start', slug: 'quick-start' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'CLI reference', slug: 'cli-reference' },
            { label: 'Configuration', slug: 'configuration' },
            { label: 'Authentication & roles', slug: 'authentication-and-roles' },
            { label: 'REST API', slug: 'rest-api' },
          ],
        },
        {
          label: 'Using Vessel',
          items: [
            { label: 'Containers', slug: 'containers' },
            { label: 'Logs & stats', slug: 'logs-and-stats' },
            { label: 'Console', slug: 'console' },
            { label: 'Volumes & networks', slug: 'volumes-and-networks' },
            { label: 'Webhooks', slug: 'webhooks' },
          ],
        },
        {
          label: 'Operating it',
          items: [
            { label: 'Security model', slug: 'security-model' },
            { label: 'Troubleshooting', slug: 'troubleshooting' },
            { label: 'Contributing', slug: 'contributing' },
          ],
        },
      ],
    }),
  ],
});
