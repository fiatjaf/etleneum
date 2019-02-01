/** @format */

var ReactDOM = require('react-dom')
var React = require('react')
var QRCode = require('react-qr-svg').QRCode

var App = function(props) {
  return React.createElement(QRCode, props, null)
}
App.displayName = 'QRCode'

module.exports = App
