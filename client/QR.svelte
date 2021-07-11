<!-- @format -->
<script>
  import {onMount, afterUpdate} from 'svelte'
  import kjua from 'kjua'

  export var value
  export var color = '#333'
  export var size = 300

  let id = parseInt(Math.random() * 100000)
  var container
  var qr

  onMount(() => {
    container = document.getElementById('c-' + id)

    qr = makeQR()
    container.appendChild(qr)
  })

  afterUpdate(() => {
    try {
      container.removeChild(qr)
      qr = makeQR()
      container.appendChild(qr)
    } catch (err) {}
  })

  function makeQR() {
    return kjua({
      rounded: 100,
      text: value.toUpperCase(),
      fill: color,
      back: 'transparent',
      size
    })
  }
</script>

<div>
  <a href="lightning:{value.toLowerCase()}" id="c-{id}" />
  <textarea readonly>{value.toLowerCase()}</textarea>
</div>

<style>
  a {
    display: block;
  }
  textarea {
    display: block;
    width: 100%;
    max-width: 100%;
    min-width: 100%;
    overflow: hidden;
    height: 4.5em;
  }
</style>
