/** @format */

var ReactDOM = require('react-dom')
var React = require('react')
var ReactJson = require('react-json-view').default

var App = function(props) {
  var actualProps = {}
  if (typeof props.src !== 'object') {
    actualProps = {
      ...props,
      src: {value: props.src}
    }
  } else {
    actualProps = props
  }

  return React.createElement(ReactJson, actualProps, null)
}
App.displayName = 'ReactJSONView'

module.exports = App
