/** @format */

const renderer = require('markdown-it')({linkify: true})
  .use(require('markdown-it-anchor'), {
    permalink: true,
    permalinkBefore: true
  })
  .use(require('markdown-it-container'), 'extra')
  .use(require('markdown-it-link-attributes'), [
    {
      pattern: /^https?:\/\//,
      attrs: {
        class: 'external-link',
        target: '_blank'
      }
    },
    {
      pattern: /^\//,
      attrs: {
        class: 'internal-link',
        onclick: `history.pushState('', '', this.href); window.dispatchEvent(new Event('popstate')); return false`
      }
    }
  ])

module.exports.markdown = function markdown(md) {
  return renderer.render(md)
}
