/** @format */

const renderer = require('markdown-it')({linkify: true})

module.exports.markdown = function markdown(md) {
  return renderer.render(md)
}
