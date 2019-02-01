/** @format */

var ReactDOM = require('react-dom')
var React = require('react')
var ReactJson = require('react-json-view').default

var App = function(props) {
  if (props.src === null || typeof props.src !== 'object') {
    return React.createElement('div', {}, '' + props.src)
  } else {
    return React.createElement(ReactJson, props, null)
  }
}
App.displayName = 'ReactJSONView'

module.exports = App
