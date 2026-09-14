#!/usr/bin/env node
// Copies the static assets that Tailwind's CLI doesn't handle into dist/.
const fs = require('fs')
const path = require('path')

const root = path.join(__dirname, '..')
const dist = path.join(root, 'dist')

fs.mkdirSync(dist, { recursive: true })
fs.mkdirSync(path.join(dist, 'fonts'), { recursive: true })

fs.copyFileSync(path.join(root, 'index.html'), path.join(dist, 'index.html'))
fs.copyFileSync(path.join(root, 'robots.txt'), path.join(dist, 'robots.txt'))
fs.copyFileSync(path.join(root, 'og-image.png'), path.join(dist, 'og-image.png'))

for (const file of fs.readdirSync(path.join(root, 'fonts'))) {
  fs.copyFileSync(path.join(root, 'fonts', file), path.join(dist, 'fonts', file))
}

console.log('copied static assets to dist/')
