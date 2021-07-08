<!-- @format -->

<script>
  import {tick, afterUpdate} from 'svelte'
  import DomJsonTree from 'dom-json-tree/src/index.js'

  export var value

  let id = parseInt(Math.random() * 100000)

  afterUpdate(async () => {
    let container = document.getElementById('j-' + id)

    while (container.children.length) {
      container.removeChild(container.children[0])
    }

    let djt = new DomJsonTree(value, container, {
      colors: {
        key: '#008080',
        type: '#546778',
        typeNumber: '#000080',
        typeString: '#dd1144',
        typeBoolean: '#000080'
      }
    })

    await tick()

    djt.render()

    await tick()

    let properties = document.querySelectorAll(`#j-${id} .djt-Property`)
    for (let i = 0; i < properties.length; i++) {
      properties[i].click()
    }
  })
</script>

<div id="j-{id}"></div>
