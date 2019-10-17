/** @format */

import Toastify from 'toastify-js'

function toast(type, html) {
  Toastify({
    text: html,
    duration: 7000,
    close: false,
    gravity: 'top',
    positionLeft: false,
    className: `toast-${type}`
  }).showToast()
}

export const info = html => toast('info', html)
export const warning = html => toast('warning', html)
export const error = html => toast('error', html)
export const success = html => toast('success', html)
